package checker

import (
	"context"
	"io/ioutil"
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
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/store"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

// CheckAnnotation contains path & position for github comment
type CheckAnnotation struct {
	Messages  []string // regexp format for comment message
	Path      string
	StartLine int
}

// TestsData contains the meta-data for a sub-test.
type TestsData struct {
	Language     string
	TestRepoPath string
	FileName     string
	Annotations  []CheckAnnotation
}

var dataSet = []TestsData{
	{"CPP", "../testdata", "sillycode.cpp", []CheckAnnotation{
		CheckAnnotation{[]string{`two`}, "sillycode.cpp", 5},
		CheckAnnotation{[]string{`explicit`}, "sillycode.cpp", 80},
	}},
	{"Go", "../testdata", "test1.go", []CheckAnnotation{
		CheckAnnotation{[]string{`\n\+\s*"bytes"`}, "test1.go", 3},
		CheckAnnotation{[]string{`\n\-\s*"bytes"`}, "test1.go", 6},
	}},
	{"Markdown", "../testdata/markdown", "hello ☺.md", []CheckAnnotation{
		{[]string{"Hello 你好"}, "hello ☺.md", 1},
		{[]string{"undefined"}, "hello ☺.md", 3},
	}},
	{"Objective-C", "../testdata/Objective-C", "sample.m", []CheckAnnotation{
		{[]string{`pool = `}, "sample.m", 3},
		{[]string{`NSLog\(`}, "sample.m", 6},
		{[]string{`\+\s+return 0;`}, "sample.m", 8},
	}},
}

func TestGenerateComments(t *testing.T) {
	Conf = config.BuildDefaultConf()

	err := InitLog(Conf)
	require.Nil(t, err)

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	currentDir := path.Dir(filename)

	for _, v := range dataSet {
		v := v
		t.Run(v.Language, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)
			assert.NotNil(assert)
			require := require.New(t)
			require.NotNil(require)

			testRepoPath := path.Join(currentDir, v.TestRepoPath)
			out, err := ioutil.ReadFile(path.Join(testRepoPath, v.FileName+".diff"))
			require.NoError(err)
			logFilePath := path.Join(testRepoPath, v.FileName+".log")
			log, err := os.Create(logFilePath)
			require.NoError(err)
			defer os.Remove(logFilePath)
			defer log.Close()

			diffs, err := diff.ParseMultiFileDiff(out)
			require.NoError(err)

			lintEnabled := LintEnabled{}
			lintEnabled.Init(testRepoPath)

			annotations, problems, err := lintIndividually(testRepoPath, diffs, lintEnabled, log)
			require.NoError(err)
			require.Equal(len(v.Annotations), problems)
			for i, check := range v.Annotations {
				assert.Equal(check.StartLine, *annotations[i].StartLine)
				assert.Equal(check.Path, *annotations[i].Path)
				for _, regexMessage := range check.Messages {
					assert.Regexp(regexMessage, *annotations[i].Message)
				}
			}
		})
	}
}

func TestGetBaseCoverage(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	Conf = config.BuildDefaultConf()
	err := InitLog(Conf)
	require.NoError(err)

	err = store.Init(":memory:")
	require.NoError(err)
	defer store.Deinit()

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
	err = cmd.Run()
	require.NoError(err)

	repoConf, err := readProjectConfig(repoPath)
	tests := repoConf.Tests
	require.NoError(err)

	author := "author"
	baseSHA := strings.TrimSpace(sha.String())
	ref := GithubRef{
		owner: "owner",
		repo:  "repo",
		Sha:   baseSHA,
	}
	baseSavedRecords, baseTestsNeedToRun := loadBaseFromStore(ref, baseSHA, tests, os.Stdout)
	assert.Empty(baseSavedRecords)
	assert.Equal(len(tests), len(baseTestsNeedToRun))
	var baseCoverage sync.Map
	err = findBaseCoverage(baseSavedRecords, baseTestsNeedToRun, repoPath, baseSHA,
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

func TestLintRepo1(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)

	currentDir := path.Dir(filename)
	repoDir := path.Join(currentDir, "../testdata/go")

	fileName := "check.go"
	out, err := ioutil.ReadFile(path.Join(repoDir, fileName+".diff"))
	require.NoError(err)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	lintEnabled := LintEnabled{}
	lintEnabled.Init(repoDir)
	Conf.Core.GolangCILint = "golangci-lint"

	var buf strings.Builder
	_, annotations, problems, err := lintRepo(context.TODO(), repoDir, diffs, lintEnabled, &buf)
	require.NoError(err)
	assert.NotEmpty(annotations)
	assert.NotZero(problems)
}

func TestLintRepo2(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)

	currentDir := path.Dir(filename)
	repoDir := path.Join(currentDir, "../testdata/Android")

	fileName := "app/src/main/AndroidManifest.xml"
	out, err := ioutil.ReadFile(path.Join(repoDir, fileName+".diff"))
	require.NoError(err)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	lintEnabled := LintEnabled{}
	lintEnabled.Init(repoDir)
	if runtime.GOOS == "windows" {
		Conf.Core.AndroidLint = "gradlew.bat lint"
	} else {
		Conf.Core.AndroidLint = "./gradlew lint"
	}

	var buf strings.Builder
	_, annotations, problems, err := lintRepo(context.TODO(), repoDir, diffs, lintEnabled, &buf)
	require.NoError(err)
	assert.NotEmpty(annotations)
	assert.NotZero(problems)
}

func TestIsOC(t *testing.T) {
	assert.False(t, isOC("abc"))
	assert.True(t, isOC("abc.mm"))
}
