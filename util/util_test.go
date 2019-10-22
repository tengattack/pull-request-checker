package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFloatPercent(t *testing.T) {
	pct, err := ParseFloatPercent("1.2% 3.14159", 64)
	require.NoError(t, err)
	assert.InDelta(t, 0.012, pct, 0.0001)

	assert.Equal(t, "58.78%", FormatFloatPercent(.5878))
}

func TestUnquote(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(`hello ☺`, Unquote(`"hello \342\230\272"`))
	assert.Equal(`hello world`, Unquote(`hello world`))
}

func TestFileExists(t *testing.T) {
	assert := assert.New(t)

	assert.False(FileExists("../testdata"))
	assert.True(FileExists("./util_test.go"))
}

func TestTruncated(t *testing.T) {
	assert := assert.New(t)

	b, s := Truncated("1200 0000 0000 0034", " ... ", 9)
	assert.Equal(true, b)
	assert.Equal("12 ... 34", s)

	b, s = Truncated("abc", "", 2)
	assert.Equal(true, b)
	assert.Equal("ac", s)

	b, s = Truncated("abc", "{,}", 2)
	assert.Equal(true, b)
	assert.Equal("{}", s)

	b, s = Truncated("abc", "", 3)
	assert.False(b)
	assert.Equal("abc", s)
}
