package util

import (
	"testing"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFileModeInDiff(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	extendedLines := []string{
		"index 13fe0dc..2332010 100644",
	}
	mode, err := ParseFileModeInDiff(extendedLines)
	require.NoError(err)
	assert.Equal(0644, mode)

	extendedLines = []string{
		"new file mode 100755",
		"index 0000000..b54741c",
	}
	mode, err = ParseFileModeInDiff(extendedLines)
	require.NoError(err)
	assert.Equal(0755, mode)
}

func TestGetTrimmedNewName(t *testing.T) {
	assert := assert.New(t)

	name, ok := GetTrimmedNewName(&diff.FileDiff{NewName: "b/name"})
	assert.True(ok)
	assert.Equal("name", name)

	name, ok = GetTrimmedNewName(&diff.FileDiff{NewName: "b/hello \342\230\272.md"})
	assert.True(ok)
	assert.Equal("hello ☺.md", name)

	name, ok = GetTrimmedNewName(&diff.FileDiff{NewName: "name"})
	assert.False(ok)
	assert.Equal("name", name)
}
