package redis

import (
	"context"
	"log"
	"strconv"
	"strings"
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
func (s *MessageQueue) Push(message string, removePrefix string, top bool) error {
	list, err := redisClient.LRange(mq.SyncChannelKey, 0, -1).Result()
	if err != nil {
		return err
	}

	found := false
	if removePrefix == "" {
		if !top {
			for _, v := range list {
				if v == message {
					found = true
					break
				}
			}
		}
	} else {
		for _, v := range list {
			if strings.HasPrefix(v, removePrefix) {
				// top message always required RPUSH
				if !top && v == message && !found {
					found = true
				} else {
					err = redisClient.LRem(mq.SyncChannelKey, 1, v).Err()
					if err != nil {
						// PASS
					}
				}
			}
		}
	}

	if top {
		// Move to top
		err = redisClient.RPush(mq.SyncChannelKey, message).Err()
	} else {
		if found {
			return nil
		}
		err = redisClient.LPush(mq.SyncChannelKey, message).Err()
	}
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

func createScript() *redis.Script {
	script := redis.NewScript(`
        local source      = tostring(KEYS[1])
        local destination = tostring(KEYS[2])
        local count       = tonumber(ARGV[1])
        local prefixs     = {}
        local t           = {}

        local sourceList = redis.call("LRANGE", source, 0, -1)

        if #sourceList <= 0 then
          return t
        end

        local function split(s, sep)
          local parts = {}
          local fpat = "(.-)" .. sep

          local i, e, cap = s:find(fpat, 1)
          local last_end = 1
          while i do
            if i ~= 1 or cap ~= "" then
              table.insert(parts, cap)
            end
            last_end = e+1
            i, e, cap = s:find(fpat, last_end)
          end
          if last_end <= #s then
            cap = s:sub(last_end)
            table.insert(parts, cap)
          end
          return parts
		end

		for i=2,#ARGV do
          local s = ARGV[i]
          local parts = split(s, "/")
          local prefix = parts[1]
          if #parts > 1 then
            prefix = prefix .. "/" .. parts[2]
          end
          local found = false
          for _, p in ipairs(prefixs) do
            if p == prefix then
              found = true
              break
            end
          end
          if not found then
            prefixs[#prefixs+1] = prefix
          end
        end

        for i = #sourceList,1,-1 do
          local s = sourceList[i]
          local parts = split(s, "/")
          local prefix = parts[1]
          if #parts > 1 then
            prefix = prefix .. "/" .. parts[2]
          end
          local found = false
          for _, p in ipairs(prefixs) do
            if p == prefix then
              found = true
              break
            end
          end
          if not found then
            prefixs[#prefixs+1] = prefix
            t[#t+1] = s
            if #t >= count then
              break
            end
          end
        end

	  if #t > 0 then
		for _, s in ipairs(t) do
		  redis.call("LREM", source, -1, s)
		  redis.call("LPUSH", destination, s)
		end
	  end

	  return t
    `)
	return script
}

func evalScript(client *redis.Client, source, destination string, n int, running []string) ([]string, error) {
	script := createScript()
	sha, err := script.Load(client).Result()
	if err != nil {
		return nil, err
	}
	args := make([]interface{}, len(running)+1)
	args[0] = strconv.Itoa(n)
	for i, s := range running {
		args[i+1] = s
	}
	ret := client.EvalSha(sha, []string{
		source,
		destination,
	}, args...)
	result, err := ret.Result()
	if err != nil {
		return nil, err
	}
	list := result.([]interface{})
	queued := make([]string, len(list))
	for i, s := range list {
		queued[i] = s.(string)
	}
	return queued, nil
}

// GetN get N messages from queue.
func (s *MessageQueue) GetN(ctx context.Context, n int, running []string) ([]string, error) {
	return evalScript(redisClient, mq.SyncChannelKey, mq.SyncPendingChannelKey, n, running)
}

func createScript2() *redis.Script {
	script := redis.NewScript(`
        local source      = tostring(KEYS[1])
        local destination = tostring(KEYS[2])
        local count       = tonumber(ARGV[1])
        local prefixs     = {}
        local within      = {}
        local t           = {}

        local sourceList = redis.call("LRANGE", source, 0, -1)

        if #sourceList <= 0 then
          return t
        end

        local function split(s, sep)
          local parts = {}
          local fpat = "(.-)" .. sep

          local i, e, cap = s:find(fpat, 1)
          local last_end = 1
          while i do
            if i ~= 1 or cap ~= "" then
              table.insert(parts, cap)
            end
            last_end = e+1
            i, e, cap = s:find(fpat, last_end)
          end
          if last_end <= #s then
            cap = s:sub(last_end)
            table.insert(parts, cap)
          end
          return parts
		end

		local within_arg = false
		for i=2,#ARGV do
		  local s = ARGV[i]
		  if s == 'WITHIN' then
			within_arg = true
		  elseif within_arg then
			within[#within+1] = s
		  else
            local parts = split(s, "/")
            local prefix = parts[1]
            if #parts > 1 then
              prefix = prefix .. "/" .. parts[2]
            end
            local found = false
            for _, p in ipairs(prefixs) do
              if p == prefix then
                found = true
                break
              end
            end
            if not found then
              prefixs[#prefixs+1] = prefix
  		    end
		  end
        end

        for i = #sourceList,1,-1 do
          local s = sourceList[i]
          local parts = split(s, "/")
          local prefix = parts[1]
          if #parts > 1 then
            prefix = prefix .. "/" .. parts[2]
		  end
          local found = false
		  if within_arg then
			for _, p in ipairs(within) do
			  if p == prefix then
			    found = true
			    break
		      end
			end
		  end
		  if (not within_arg) or (within_arg and found) then
  		    found = false
            for _, p in ipairs(prefixs) do
              if p == prefix then
                found = true
                break
              end
            end
            if not found then
              prefixs[#prefixs+1] = prefix
              t[#t+1] = s
              if #t >= count then
                break
              end
  		    end
		  end
	    end

	  if #t > 0 then
		for _, s in ipairs(t) do
		  redis.call("LREM", source, -1, s)
		  redis.call("LPUSH", destination, s)
		end
	  end

	  return t
    `)
	return script
}

func evalScript2(client *redis.Client, source, destination string, n int, running []string, within []string) ([]string, error) {
	script := createScript2()
	sha, err := script.Load(client).Result()
	if err != nil {
		return nil, err
	}
	args := make([]interface{}, 0, len(running)+2+len(within))
	args = append(args, strconv.Itoa(n))
	for _, s := range running {
		args = append(args, s)
	}
	if within != nil {
		args = append(args, "WITHIN")
		for _, p := range within {
			args = append(args, p)
		}
	}
	ret := client.EvalSha(sha, []string{
		source,
		destination,
	}, args...)
	result, err := ret.Result()
	if err != nil {
		return nil, err
	}
	list := result.([]interface{})
	queued := make([]string, len(list))
	for i, s := range list {
		queued[i] = s.(string)
	}
	return queued, nil
}

// GetNWithin get N messages from queue within list.
func (s *MessageQueue) GetNWithin(ctx context.Context, n int, running []string, within []string) ([]string, error) {
	return evalScript2(redisClient, mq.SyncChannelKey, mq.SyncPendingChannelKey, n, running, within)
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
// TODO: add test
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
