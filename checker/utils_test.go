package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/config"
)

func TestSearchGithubPR(t *testing.T) {
	Conf = config.BuildDefaultConf()
	InitLog()
	require := require.New(t)
	assert := assert.New(t)
	i, e := searchGithubPR("tengattack/unified-ci", "7988bac704d600a86bd29149c569c788f0d7cd92")
	require.NoError(e)
	assert.EqualValues(23, i)
}
