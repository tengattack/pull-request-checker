package checker

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/common"
)

func TestHandleMessage(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	err := common.InitJWTClient(common.Conf.GitHub.AppID, common.Conf.GitHub.PrivateKey)
	require.NoError(err)

	start := time.Now()
	duration := 15 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	err = HandleMessage(ctx, "tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2")
	require.NoError(err)
	assert.True(time.Since(start) < 20*time.Second)
	assert.True(time.Since(start) > 15*time.Second)
}

func TestFilterLints(t *testing.T) {
	assert := assert.New(t)

	file := "sdk/v2/x"
	annotations, filtered := filterLints([]string{"sdk/**"}, []*github.CheckRunAnnotation{
		&github.CheckRunAnnotation{Path: &file},
	})
	assert.Empty(annotations)
	assert.Equal(1, filtered)
}
