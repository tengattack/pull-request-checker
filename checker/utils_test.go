package checker

import (
	"context"
	"path"
	"runtime"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/config"
)

func TestSearchGithubPR(t *testing.T) {
	Conf = config.BuildDefaultConf()
	InitLog()
	require := require.New(t)
	assert := assert.New(t)

	client := github.NewClient(nil)
	i, err := searchGithubPR(context.Background(), client, "tengattack/unified-ci", "7988bac704d600a86bd29149c569c788f0d7cd92")
	require.NoError(err)
	assert.EqualValues(23, i)
}

func TestGetTests(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	tests, err := getTests(currentDir)
	require.NoError(err)
	assert.Empty(tests)

	tests, err = getTests(currentDir + "/../")
	require.NoError(err)
	assert.True(len(tests) > 0)
}
