package redis

import (
	"log"
	"strconv"

	"../../config"
	"../../mq"

	"gopkg.in/redis.v5"
)

//
var redisClient *redis.Client

// New func implements the storage interface for mpush
func New(config config.Config) *MessageQueue {
	return &MessageQueue{
		config: config,
	}
}

// MessageQueue is interface structure
type MessageQueue struct {
	config config.Config
}

// Init client storage.
func (s *MessageQueue) Init() error {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     s.config.MessageQueue.Redis.Addr,
		Password: s.config.MessageQueue.Redis.Password,
		DB:       s.config.MessageQueue.Redis.DB,
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
func (s *MessageQueue) Subscribe() (string, error) {
	return redisClient.BRPopLPush(mq.SyncChannelKey, mq.SyncPendingChannelKey, 0).Result()
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
