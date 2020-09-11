package util

import (
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadProjectConfig(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	repoConf, err := ReadProjectConfig(currentDir)
	require.NoError(err)
	assert.Empty(repoConf.Tests)

	repoConf, err = ReadProjectConfig(currentDir + "/../")
	require.NoError(err)
	assert.True(len(repoConf.Tests) > 0)
	assert.Equal([]string{
		"testdata/**",
		"sdk/**",
	}, repoConf.IgnorePatterns)
}
