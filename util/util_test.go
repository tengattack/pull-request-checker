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
