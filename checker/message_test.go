package checker

import (
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/config"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

// CheckComment contains path & position for github comment
type CheckComment struct {
	Messages []string // regexp format for comment message
	Path     string
	// The position in the diff where you want to add a review comment.
	// Note this value is not the same as the line number in the file.
	// The position value equals the number of lines down from the first "@@" hunk header in the file you want to
	// add a comment. The line just below the "@@" line is position 1, the next line is position 2, and so on.
	// The position in the diff continues to increase through lines of whitespace and additional hunks until the
	// beginning of a new file.
	// See more information: https://developer.github.com/v3/pulls/comments/
	Position int // offset in the unified diff
}

// TestsData contains the meta-data for a sub-test.
type TestsData struct {
	Language      string
	TestRepoPath  string
	FileName      string
	CheckComments []CheckComment
}

var dataSet = []TestsData{
	{"CPP", "../tests", "sillycode.cpp", []CheckComment{
		CheckComment{[]string{`two`}, "sillycode.cpp", 7},
		CheckComment{[]string{`explicit`}, "sillycode.cpp", 16},
	}},
	{"Go", "../tests", "test1.go", []CheckComment{
		CheckComment{[]string{`\n\+\s*"bytes"`}, "test1.go", 2},
		CheckComment{[]string{`\n\-\s*"bytes"`}, "test1.go", 5},
	}},
	{"Markdown", "../tests/markdown", "hello ☺.md", []CheckComment{
		{[]string{"Hello 你好"}, "hello ☺.md", 2},
		{[]string{"undefined"}, "hello ☺.md", 5},
	}},
}

func TestGenerateComments(t *testing.T) {
	conf, err := config.LoadConfig("../config.yml")
	require.Nil(t, err)
	Conf = conf

	err = InitLog()
	require.Nil(t, err)

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	currentDir := path.Dir(filename)

	for _, v := range dataSet {
		t.Run(v.Language, func(t *testing.T) {
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

			comments, problems, err := GenerateComments(testRepoPath, diffs, &lintEnabled, log)
			require.NoError(err)
			require.Equal(len(v.CheckComments), problems)
			for i, check := range v.CheckComments {
				assert.Equal(check.Position, comments[i].Position)
				assert.Equal(check.Path, comments[i].Path)
				for _, regexMessage := range check.Messages {
					assert.Regexp(regexMessage, comments[i].Body)
				}
			}
		})
	}
}
