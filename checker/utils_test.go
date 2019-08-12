package checker

import (
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestNewShellParser(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	parser := NewShellParser(currentDir)
	require.NotNil(parser)

	words, err := parser.Parse("echo $PWD $PROJECT_NAME")
	require.NoError(err)
	assert.Equal([]string{"echo", currentDir, "checker"}, words)
}
