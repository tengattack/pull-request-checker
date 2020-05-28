package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ok, err := ScanPKG(Golang, "test", "./testdata", "go.sum")
	require.NoError(err)
	assert.True(ok)
}
