package redis

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/tengattack/unified-ci/mq"
	"gopkg.in/redis.v5"
)

//
var redisClient *redis.Client

// Config redis message queue
type Config struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

// New func implements the storage interface for mpush
func New(config Config) *MessageQueue {
	return &MessageQueue{
		config: config,
	}
}

// MessageQueue is interface structure
type MessageQueue struct {
	config Config
}

// Init client storage.
func (s *MessageQueue) Init() error {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     s.config.Addr,
		Password: s.config.Password,
		DB:       s.config.DB,
		PoolSize: s.config.PoolSize,
	})

	_, err := redisClient.Ping().Result()

	if err != nil {
		// redis server error
		log.Println("Can't connect redis server: " + err.Error())

		return err
	}

	return nil
}

// Reset client message queue.
func (s *MessageQueue) Reset() {
	redisClient.Del(mq.SyncChannelKey)
}

// Push message to queue
func (s *MessageQueue) Push(message string) error {
	_, err := redisClient.LPush(mq.SyncChannelKey, message).Result()
	return err
}

// Subscribe message from queue.
func (s *MessageQueue) Subscribe(ctx context.Context) (string, error) {
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		msg, err := redisClient.BRPopLPush(mq.SyncChannelKey, mq.SyncPendingChannelKey, 5*time.Second).Result()
		// If timeout is reached, a redis.Nil will be returned
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return "", err
		}
		return msg, nil
	}
}

// Finish message processing
func (s *MessageQueue) Finish(message string) error {
	// remove the message in error channel
	_, err := redisClient.LRem(mq.SyncPendingChannelKey, 1, message).Result()
	redisClient.HDel(mq.SyncRetriesChannelKey, message).Result()
	return err
}

// Error mark message as error
func (s *MessageQueue) Error(message string) error {
	// add the message to error channel
	_, err := redisClient.LPush(mq.SyncErrorChannelKey, message).Result()
	if err != nil {
		return err
	}
	_, err = redisClient.LRem(mq.SyncPendingChannelKey, 1, message).Result()
	if err != nil {
		return err
	}
	_, err = redisClient.HIncrBy(mq.SyncRetriesChannelKey, message, 1).Result()
	return err
}

// MoveAllPendingToError moves all pending messages to error channel
func (s *MessageQueue) MoveAllPendingToError() (int, error) {
	count := 0
	for {
		_, err := redisClient.RPopLPush(mq.SyncPendingChannelKey, mq.SyncErrorChannelKey).Result()
		if err != nil {
			break
		}
		count++
	}
	return count, nil
}

// MoveErrorToPending moves a pending message to error channel
func (s *MessageQueue) MoveErrorToPending() (string, error) {
	return redisClient.RPopLPush(mq.SyncErrorChannelKey, mq.SyncPendingChannelKey).Result()
}

// GetErrorTimes returns the message error times
func (s *MessageQueue) GetErrorTimes(message string) (int64, error) {
	times, err := redisClient.HGet(mq.SyncRetriesChannelKey, message).Result()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(times, 10, 64)
}

// Retry moves the message from pending channel to queue
func (s *MessageQueue) Retry(message string) error {
	// add the message to queue
	_, err := redisClient.LPush(mq.SyncChannelKey, message).Result()
	if err != nil {
		return err
	}
	_, err = redisClient.LRem(mq.SyncPendingChannelKey, 1, message).Result()
	return err
}

// Exists checks if message is in the queue
func (s *MessageQueue) Exists(message string) (bool, error) {
	// SyncChannelKey
	list, err := redisClient.LRange(mq.SyncChannelKey, 0, -1).Result()
	if err != nil {
		return false, err
	}
	for _, v := range list {
		if v == message {
			return true, nil
		}
	}

	// SyncPendingChannelKey
	list, err = redisClient.LRange(mq.SyncPendingChannelKey, 0, -1).Result()
	if err != nil {
		return false, err
	}
	for _, v := range list {
		if v == message {
			return true, nil
		}
	}

	// SyncErrorChannelKey
	list, err = redisClient.LRange(mq.SyncErrorChannelKey, 0, -1).Result()
	if err != nil {
		return false, err
	}
	for _, v := range list {
		if v == message {
			return true, nil
		}
	}
	return false, nil
}
