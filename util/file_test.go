package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeadFile(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	assert.Panics(func() {
		_, _ = HeadFile("../testdata/lines", 0)
	})

	lines, err := HeadFile("../testdata/lines", 1)
	require.NoError(err)
	assert.Len(lines, 1)
	assert.Equal("a", lines[0])

	lines, err = HeadFile("../testdata/lines", 3)
	require.NoError(err)
	assert.Len(lines, 2)
	assert.Equal("a", lines[0])
	assert.Equal("b", lines[1])
}

func TestFileExists(t *testing.T) {
	assert := assert.New(t)

	assert.False(FileExists("../testdata"))
	assert.False(FileExists("./filenoexists_test.go"))
	assert.True(FileExists("./util_test.go"))
}
