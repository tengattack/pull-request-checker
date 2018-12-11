package checker

import (
	"context"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGotest(t *testing.T) {
	_, filepath, _, _ := runtime.Caller(0)

	out, err := Gotest(context.Background(), path.Dir(filepath)+"/../testdata/go")
	assert.NoError(t, err)
	assert.NotEmpty(t, out)
}
