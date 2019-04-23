package checker

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime"
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
}

func TestGenerateComments(t *testing.T) {
	Conf = config.BuildDefaultConf()

	err := InitLog()
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

			annotations, problems, err := GenerateAnnotations(testRepoPath, diffs, lintEnabled, log)
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
	err := InitLog()
	require.NoError(err)

	err = store.Init(":memory:")
	require.NoError(err)
	defer store.Deinit()

	_, filename, _, _ := runtime.Caller(0)
	repoPath := path.Join(path.Dir(filename), "/../testdata/go")

	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "add", ".gitignore")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-am", "init")
	cmd.Dir = repoPath
	cmd.Run()

	tests, err := getTests(repoPath)
	require.NoError(err)

	author := "author"
	baseCoverage, err := findBaseCoverage(repoPath, tests, &github.PullRequest{
		Head: &github.PullRequestBranch{
			User: &github.User{
				Login: &author,
			},
		},
	}, GithubRef{
		owner: "owner",
		repo:  "repo",
		Sha:   "sha",
	}, ioutil.Discard)
	require.NoError(err)
	value, _ := baseCoverage.Load("go")
	coverage, _ := value.(string)
	assert.Regexp(percentageRegexp, coverage)
}
