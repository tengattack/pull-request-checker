package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFloatPercent(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	pct, norm, err := ParseFloatPercent("1.2% 3.14159", 64)
	require.NoError(err)
	assert.Equal("1.2%", norm)
	assert.InDelta(0.012, pct, 0.0001)

	pct, norm, err = ParseFloatPercent("12.3", 64)
	require.NoError(err)
	assert.Equal("12.3%", norm)
	assert.InDelta(0.123, pct, 0.0001)

	assert.Equal("58.78%", FormatFloatPercent(.5878))
}

func TestUnquote(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(`hello ☺`, Unquote(`"hello \342\230\272"`))
	assert.Equal(`hello world`, Unquote(`hello world`))
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

func TestMatchAny(t *testing.T) {
	assert := assert.New(t)

	assert.True(MatchAny([]string{"sdk/**"}, "sdk/v2/x"))
	assert.False(MatchAny([]string{"sdk/*"}, "sdk/v2/x"))
}
