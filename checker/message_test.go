package checker

import (
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	// see more information: https://developer.github.com/v3/pulls/comments/
	Position int
}

func TestGenerateCommentsGo(t *testing.T) {
	assert := assert.New(t)
	assert.NotNil(assert)
	require := require.New(t)
	require.NotNil(require)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)

	checkComments := []CheckComment{
		CheckComment{[]string{`\n\+\s*"bytes"`}, "test1.go", 2},
		CheckComment{[]string{`\n\-\s*"bytes"`}, "test1.go", 5},
	}

	testRepoPath := path.Join(path.Dir(filename), "../tests")
	goDiffPath := path.Join(testRepoPath, "test1.go.diff")
	out, err := ioutil.ReadFile(goDiffPath)
	require.NoError(err)

	// log file
	logFilePath := path.Join(testRepoPath, "test1.go.log")
	log, err := os.Create(logFilePath)
	require.NoError(err)
	defer os.Remove(logFilePath)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	lintEnabled := LintEnabled{}
	lintEnabled.Init(testRepoPath)

	comments, problems, err := GenerateComments(testRepoPath, diffs, &lintEnabled, log)
	require.NoError(err)
	require.Equal(len(checkComments), problems)
	for i, check := range checkComments {
		assert.Equal(check.Position, comments[i].Position)
		assert.Equal(check.Path, comments[i].Path)
		for _, regexMessage := range check.Messages {
			assert.Regexp(regexMessage, comments[i].Body)
		}
	}
}
