package redis

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/mq"
)

var MQ *MessageQueue

func TestMain(m *testing.M) {
	MQ = New(Config{Addr: "127.0.0.1:6379"})
	err := MQ.Init()
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestReset(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	require.NotPanics(func() { MQ.Reset() })
	result, err := redisClient.Exists(mq.SyncChannelKey).Result()
	require.NoError(err)
	assert.False(result)
}

func TestPush(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	MQ.Reset()

	err := MQ.Push("tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2", "tengattack/playground/pull/2/")
	require.NoError(err)

	err = MQ.Push("tengattack/playground/pull/3/commits/2941c50a0126ea878203ac72272ea46aeee148f6", "tengattack/playground/pull/3/")
	require.NoError(err)

	err = MQ.Push("tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c", "tengattack/playground/pull/3/")
	require.NoError(err)

	list, err := redisClient.LRange(mq.SyncChannelKey, 0, -1).Result()
	require.NoError(err)
	assert.Equal([]string{
		"tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c",
		"tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2",
	}, list)

	err = MQ.Push("tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c", "")
	require.NoError(err)

	list, err = redisClient.LRange(mq.SyncChannelKey, 0, -1).Result()
	require.NoError(err)
	assert.Equal([]string{
		"tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c",
		"tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2",
	}, list)
}

func TestGetN(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	// --------
	MQ.Reset()
	err := redisClient.LPush(mq.SyncChannelKey,
		"tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2",
		"tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c",
		"tengattack/foo1/pull/1/commits/a",
		"tengattack/foo2/pull/2/commits/b",
		"tengattack/foo2/pull/3/commits/c",
	).Err()
	require.NoError(err)

	jobs, err := MQ.GetN(context.Background(), 2, []string{})
	require.NoError(err)
	assert.Equal([]string{
		"tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2",
		"tengattack/foo1/pull/1/commits/a",
	}, jobs)

	// --------
	MQ.Reset()
	err = redisClient.LPush(mq.SyncChannelKey,
		"tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2",
		"tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c",
		"tengattack/foo1/pull/1/commits/a",
		"tengattack/foo2/pull/2/commits/b",
		"tengattack/foo2/pull/3/commits/c",
	).Err()
	require.NoError(err)

	jobs, err = MQ.GetN(context.Background(), 2, []string{"tengattack/playground"})
	require.NoError(err)
	assert.Equal([]string{
		"tengattack/foo1/pull/1/commits/a",
		"tengattack/foo2/pull/2/commits/b",
	}, jobs)
}

func TestGetNWithin(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	// --------
	MQ.Reset()
	err := redisClient.LPush(mq.SyncChannelKey,
		"tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2",
		"tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c",
		"tengattack/foo1/pull/1/commits/a",
		"tengattack/foo2/pull/2/commits/b",
		"tengattack/foo2/pull/3/commits/c",
	).Err()
	require.NoError(err)

	jobs, err := MQ.GetNWithin(context.Background(), 2, []string{}, []string{})
	require.NoError(err)
	assert.Equal([]string{}, jobs)

	jobs, err = MQ.GetNWithin(context.Background(), 2, []string{}, nil)
	require.NoError(err)
	assert.Equal([]string{
		"tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2",
		"tengattack/foo1/pull/1/commits/a",
	}, jobs)

	jobs, err = MQ.GetNWithin(context.Background(), 2, []string{}, []string{"tengattack/foo2"})
	require.NoError(err)
	assert.Equal([]string{"tengattack/foo2/pull/2/commits/b"}, jobs)

	// --------
	MQ.Reset()
	err = redisClient.LPush(mq.SyncChannelKey,
		"tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2",
		"tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c",
		"tengattack/foo1/pull/1/commits/a",
		"tengattack/foo2/pull/2/commits/b",
		"tengattack/foo2/pull/3/commits/c",
	).Err()
	require.NoError(err)

	jobs, err = MQ.GetNWithin(context.Background(), 2, []string{"tengattack/foo1/pull/1/commits/aa"}, []string{"tengattack/foo2"})
	require.NoError(err)
	assert.Equal([]string{"tengattack/foo2/pull/2/commits/b"}, jobs)

	jobs, err = MQ.GetNWithin(context.Background(), 2, []string{}, []string{"tengattack/foo1", "tengattack/foo2"})
	require.NoError(err)
	assert.Equal([]string{"tengattack/foo1/pull/1/commits/a", "tengattack/foo2/pull/3/commits/c"}, jobs)
}
