package checker

import (
	"errors"
	"math"
	"time"

	"../mq/redis"
)

// InitMessageQueue for initialize message queue
func InitMessageQueue() error {
	LogAccess.Debug("Init Message Queue Engine as ", Conf.MessageQueue.Engine)
	switch Conf.MessageQueue.Engine {
	case "redis":
		MQ = redis.New(Conf)
	default:
		LogError.Error("mq error: can't find mq driver")
		return errors.New("can't find mq driver")
	}

	if err := MQ.Init(); err != nil {
		LogError.Error("mq error: " + err.Error())

		return err
	}

	return nil
}

// StartMessageSubscription for main message subscription and process message
func StartMessageSubscription() {
	LogAccess.Info("Start Message Subscription")

	for {
		LogAccess.Info("Waiting for message...")
		message, err := MQ.Subscribe()
		if err != nil {
			LogError.Error("mq subscribe error: " + err.Error())
		}
		LogAccess.Info("Got message: " + message)

		err = HandleMessage(message)
		if err != nil {
			LogError.Error("handle message error: " + err.Error())
			err = MQ.Error(message)
			if err != nil {
				LogError.Error("mark message error failed: " + err.Error())
			}
			continue
		}

		err = MQ.Finish(message)
		if err != nil {
			LogError.Error("mq finish error: " + err.Error())
		}
	}
}

// RetryErrorMessages helps retry error messages
func RetryErrorMessages() {
	// move pending message to error channel
	count, _ := MQ.MoveAllPendingToError()
	if count > 0 {
		LogAccess.Infof("Move %d pending message(s) to error channel", count)
	}

	for {
		time.Sleep(6 * time.Second)
		s, err := MQ.MoveErrorToPending()
		if err != nil || len(s) <= 0 {
			continue
		}
		retries, _ := MQ.GetErrorTimes(s)
		LogAccess.Infof("Retry message: '%s', retries: %d", s, retries)
		if retries <= Conf.Core.MaxRetries {
			go retryMessage(s, retries)
		}
	}
}

// Analytic (Binet's formula)
func fibonacciBinet(num int64) int64 {
	n := float64(num)
	return int64(((math.Pow(((1+math.Sqrt(5))/2), n) - math.Pow(1-((1+math.Sqrt(5))/2), n)) / math.Sqrt(5)) + 0.5)
}

func retryMessage(message string, retries int64) {
	time.Sleep(time.Duration(fibonacciBinet(retries)*60) * time.Second)
	MQ.Retry(message)
}
