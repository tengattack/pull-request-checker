package checker

import (
	"context"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCarry(t *testing.T) {
	_, filepath, _, _ := runtime.Caller(0)

	out, err := carry(context.Background(), path.Dir(filepath)+"/../testdata/go", "go test", "./...")
	assert.NoError(t, err)
	assert.NotEmpty(t, out)
}
