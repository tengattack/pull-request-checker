package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFibonacciBinet(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(int64(1), FibonacciBinet(1))
	assert.Equal(int64(1), FibonacciBinet(2))
	assert.Equal(int64(5), FibonacciBinet(5))
	assert.Equal(int64(55), FibonacciBinet(10))
	assert.Equal(int64(6765), FibonacciBinet(20))
}
