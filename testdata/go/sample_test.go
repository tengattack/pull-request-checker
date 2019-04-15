package sample

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type Test struct {
	in  int
	out string
}

var tests = []Test{
	{-1, "negative"},
	{5, "small"},
}

func TestSize(t *testing.T) {
	for _, test := range tests {
		size := size(test.in)
		assert.Equal(t, size, test.out)
	}
}
