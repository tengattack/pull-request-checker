package checker_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tengattack/unified-ci/checker"
)

func TestAPIDoc(t *testing.T) {
	assert := assert.New(t)

	output, err := checker.APIDoc(context.Background(), "../testdata/go")
	assert.Error(err)
	assert.NotEmpty(output)
}
