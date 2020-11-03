package tester

import (
	"context"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/util"
)

func TestGetBaseCoverage(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	_, filename, _, _ := runtime.Caller(0)
	repoPath := path.Join(path.Dir(filename), "/../testdata/go")

	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	_ = cmd.Run()
	defer os.RemoveAll(path.Join(repoPath, ".git/"))

	cmd = exec.Command("git", "add", ".gitignore")
	cmd.Dir = repoPath
	_ = cmd.Run()

	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = repoPath
	_ = cmd.Run()

	cmd = exec.Command("git", "-c", "user.name=test", "-c", "user.email=user@test.com", "commit", "-am", "init")
	cmd.Dir = repoPath
	_ = cmd.Run()

	var sha strings.Builder
	cmd = exec.Command("git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = repoPath
	cmd.Stdout = &sha
	err := cmd.Run()
	require.NoError(err)

	repoConf, err := util.ReadProjectConfig(repoPath)
	tests := repoConf.Tests
	require.NoError(err)

	author := "author"
	baseSHA := strings.TrimSpace(sha.String())
	ref := common.GithubRef{
		Owner:    "owner",
		RepoName: "repo",

		Sha: baseSHA,
	}
	baseSavedRecords, baseTestsNeedToRun := loadBaseFromStore(ref, baseSHA, tests, os.Stdout)
	assert.Empty(baseSavedRecords)
	assert.Equal(len(tests), len(baseTestsNeedToRun))
	var baseCoverage sync.Map
	err = findBaseCoverage(context.TODO(), baseSavedRecords, baseTestsNeedToRun, repoPath, baseSHA,
		&github.PullRequest{
			Head: &github.PullRequestBranch{
				User: &github.User{
					Login: &author,
				},
			},
		}, ref, os.Stdout, &baseCoverage)
	require.NoError(err)

	value, _ := baseCoverage.Load("go")
	coverage, _ := value.(string)
	assert.Regexp(percentageRegexp, coverage)

	baseSavedRecords, baseTestsNeedToRun = loadBaseFromStore(ref, baseSHA, tests, os.Stdout)
	assert.Empty(baseTestsNeedToRun)
	assert.True(len(baseSavedRecords) == 1)
	assert.True(*baseSavedRecords[0].Coverage > 0)
}
