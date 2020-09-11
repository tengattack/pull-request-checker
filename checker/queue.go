package checker

import (
	"context"
	"errors"
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

	for {
		select {
		case <-ctx.Done():
			common.LogAccess.Warn("StartMessageSubscription canceled.")
			return
		default:
		}
		common.LogAccess.Info("Waiting for message...")
		message, err := common.MQ.Subscribe(ctx)
		if err != nil && err != context.Canceled {
			common.LogError.Error("mq subscribe error: " + err.Error())
			continue
		}
		if message == "" {
			continue
		}
		common.LogAccess.Info("Got message: " + message)

		err = HandleMessage(ctx, message)
		if err != nil {
			common.LogError.Error("handle message error: " + err.Error())
			err = common.MQ.Error(message)
			if err != nil {
				common.LogError.Error("mark message error failed: " + err.Error())
			}
			continue
		}

		err = common.MQ.Finish(message)
		if err != nil {
			common.LogError.Error("mq finish error: " + err.Error())
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
