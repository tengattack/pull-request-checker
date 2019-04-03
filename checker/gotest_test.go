package checker

import (
	"context"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	shellwords "github.com/tengattack/go-shellwords"
)

func TestCarry(t *testing.T) {
	assert := assert.New(t)
	_, filepath, _, _ := runtime.Caller(0)
	curDir := path.Dir(filepath)

	parser := shellwords.NewParser()
	out, err := carry(context.Background(), parser, curDir+"/../testdata/go", "go test ./...")
	assert.NoError(err)
	assert.NotEmpty(out)
}
