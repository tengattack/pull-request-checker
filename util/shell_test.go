package util

import (
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/common"
)

func TestNewShellParser(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	ref := common.GithubRef{
		CheckType: common.CheckTypeBranch,
		CheckRef:  "stable",
	}
	parser := NewShellParser(currentDir, ref)
	require.NotNil(parser)

	words, err := parser.Parse("echo $PWD $PROJECT_NAME $CI_CHECK_TYPE $CI_CHECK_REF")
	require.NoError(err)
	assert.Equal([]string{"echo", currentDir, "util", common.CheckTypeBranch, "stable"}, words)
}
