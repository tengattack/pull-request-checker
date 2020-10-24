package checker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/mq/redis"
)

// InitMessageQueue for initialize message queue
func InitMessageQueue() error {
	common.LogAccess.Debug("Init Message Queue Engine as ", common.Conf.MessageQueue.Engine)
	switch common.Conf.MessageQueue.Engine {
	case "redis":
		common.MQ = redis.New(common.Conf.MessageQueue.Redis)
	default:
		common.LogError.Error("mq error: can't find mq driver")
		return errors.New("can't find mq driver")
	}

	if err := common.MQ.Init(); err != nil {
		common.LogError.Error("mq error: " + err.Error())

		return err
	}

	return nil
}

// StartMessageSubscription for main message subscription and process message
func StartMessageSubscription(ctx context.Context) {
	common.LogAccess.Info("Start Message Subscription")

	maxQueue := common.Conf.Concurrency.Queue
	if maxQueue < 1 {
		maxQueue = 1
	}
	pending := make(chan int, maxQueue)
	var running int64

	for {
		select {
		case <-ctx.Done():
			common.LogAccess.Warn("StartMessageSubscription canceled.")
			return
		default:
		}

		pending <- 0
		common.LogAccess.Info("Waiting for message...")
		var msgsMap sync.Map
		var messages []string
		var err error
		for {
			select {
			case <-ctx.Done():
				common.LogAccess.Warn("StartMessageSubscription canceled.")
				return
			case <-time.After(5 * time.Second):
			}
			var runningQueue []string
			msgsMap.Range(func(key, value interface{}) bool {
				runningQueue = append(runningQueue, key.(string))
				return true
			})
			messages, err = common.MQ.GetN(ctx, maxQueue-int(atomic.LoadInt64(&running)), runningQueue)
			if err != nil {
				break
			}
			if len(messages) > 0 {
				break
			}
		}
		<-pending
		if err != nil && err != context.Canceled {
			common.LogError.Error("mq subscribe error: " + err.Error())
			continue
		}
		if len(messages) <= 0 {
			continue
		}
		for _, message := range messages {
			pending <- 0
			atomic.AddInt64(&running, 1)
			msgsMap.Store(message, 1)
			common.LogAccess.Info("Got message: " + message)

			go func(message string) {
				defer func() {
					msgsMap.Delete(message)
					atomic.AddInt64(&running, -1)
					<-pending
				}()
				err := HandleMessage(ctx, message)
				if err != nil {
					common.LogError.Error("handle message error: " + err.Error())
					err = common.MQ.Error(message)
					if err != nil {
						common.LogError.Error("mark message error failed: " + err.Error())
					}
					return
				}

				err = common.MQ.Finish(message)
				if err != nil {
					common.LogError.Error("mq finish error: " + err.Error())
				}
			}(message)
		}
	}
}

// RetryErrorMessages helps retry error messages
func RetryErrorMessages(ctx context.Context) {
	// move pending message to error channel
	count, _ := common.MQ.MoveAllPendingToError()
	if count > 0 {
		common.LogAccess.Infof("Move %d pending message(s) to error channel", count)
	}

	for {
		select {
		case <-ctx.Done():
			common.LogAccess.Warn("RetryErrorMessages canceled.")
			return
		case <-time.After(60 * time.Second):
		}
		s, err := common.MQ.MoveErrorToPending()
		if err != nil || len(s) <= 0 {
			continue
		}
		retries, _ := common.MQ.GetErrorTimes(s)
		common.LogAccess.Infof("Retry message: '%s', retries: %d", s, retries)
		if retries <= common.Conf.Core.MaxRetries {
			go retryMessage(s, retries)
		}
	}
}

func retryMessage(message string, retries int64) {
	time.Sleep(time.Duration(FibonacciBinet(retries)*60) * time.Second)
	common.MQ.Retry(message)
}
