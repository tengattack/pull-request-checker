package sample

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDivision(t *testing.T) {
	assert := assert.New(t)
	r := division(2, 1)
	assert.Equal(2, r)
}
