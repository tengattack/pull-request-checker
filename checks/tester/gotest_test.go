package tester

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strings"
	"testing"

	shellwords "github.com/mattn/go-shellwords"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/util"
)

var (
	percentageRegexp = regexp.MustCompile(`[-+]?(?:\d*\.\d+|\d+)%`)
)

func TestCoverRegex(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filepath, _, _ := runtime.Caller(0)
	curDir := path.Dir(filepath)
	repo := curDir + "/../../testdata/go"

	repoConf, err := util.ReadProjectConfig(repo)
	tests := repoConf.Tests
	require.NoError(err)
	test, ok := tests["go"]
	require.True(ok)

	parser := shellwords.NewParser()
	parser.ParseEnv = true
	parser.ParseBacktick = true
	parser.Dir = repo

	var result string
	var output string
	var pct float64
	log := new(strings.Builder)
	for _, cmd := range test.Cmds {
		out := new(strings.Builder)
		w := io.MultiWriter(log, out)
		errCmd := carry(context.Background(), parser, repo, cmd, w)
		assert.NoError(errCmd)
		output += ("\n" + out.String())
	}

	if test.Coverage != "" {
		result, pct, err = parseCoverage(test.Coverage, output)
		assert.NoError(err)
	}
	assert.Regexp(percentageRegexp, result)
	assert.True(pct > 0)
}

func TestDeltaTest(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filepath, _, _ := runtime.Caller(0)
	curDir := path.Dir(filepath)
	repo := curDir + "/../../testdata/go"
	ref := common.GithubRef{
		BaseSha: "master",
	}
	parser := util.NewShellParser(repo, ref)
	words, err := parser.Parse(common.Conf.Core.GitCommand)
	assert.NoError(err)

	// 初始化项目
	require.NoError(os.RemoveAll(path.Join(repo, ".git")))
	// git init
	assert.NoError(runGitCommand(repo, words, []string{"init"}))
	// git add .
	assert.NoError(runGitCommand(repo, words, []string{"add", "."}))
	// git reset HEAD -- delta_sample.go
	assert.NoError(runGitCommand(repo, words, []string{"reset", "HEAD", "--", "delta_sample.go"}))
	// git reset HEAD -- delta_sample_test.go
	assert.NoError(runGitCommand(repo, words, []string{"reset", "HEAD", "--", "delta_sample_test.go"}))
	// git commit -m "master"
	assert.NoError(runGitCommand(repo, words, []string{"commit", "-m", "'master'"}))
	// git checkout -b delta
	assert.NoError(runGitCommand(repo, words, []string{"checkout", "-b", "delta"}))
	// git add .
	assert.NoError(runGitCommand(repo, words, []string{"add", "."}))
	// git commit -m "delta"
	assert.NoError(runGitCommand(repo, words, []string{"commit", "-m", "'delta'"}))

	// 执行增量测试的命令
	repoConf, err := util.ReadProjectConfig(repo)
	require.NoError(err)
	tests := repoConf.Tests
	test, ok := tests["go"]
	require.True(ok)

	var result string
	var output string
	var pct float64
	log := new(strings.Builder)
	for _, cmd := range test.Cmds {
		out := new(strings.Builder)
		w := io.MultiWriter(log, out)
		errCmd := carry(context.Background(), parser, repo, cmd, w)
		assert.NoError(errCmd)
		output += ("\n" + out.String())
	}
	if test.Coverage != "" {
		result, pct, err = parseCoverage(test.DeltaCoverage, output)
		assert.NoError(err)
	}
	assert.Regexp(percentageRegexp, result)
	assert.Equal("60%", result)
	assert.Equal(0.6, pct)
}

func runGitCommand(dir string, command, args []string) error {
	gitCmds := make([]string, len(command))
	copy(gitCmds, command)
	gitCmds = append(gitCmds, args...)
	cmds := exec.CommandContext(context.Background(), gitCmds[0], gitCmds[1:]...)
	cmds.Dir = dir
	return cmds.Run()
}
