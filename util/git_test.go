package util

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/common"
)

func TestRunGitCommand(t *testing.T) {
	require := require.New(t)

	common.Conf.Core.GitCommand = "git"

	require.NoError(RunGitCommand(common.GithubRef{}, ".", []string{"status"}, nil))
}
