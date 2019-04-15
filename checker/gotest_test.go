package checker

import (
	"context"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	shellwords "github.com/tengattack/go-shellwords"
)

func TestCoverRegex(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filepath, _, _ := runtime.Caller(0)
	curDir := path.Dir(filepath)
	repo := curDir + "/../testdata/go"

	tests, err := getTests(repo)
	require.NoError(err)
	tasks := tests["go"]
	require.Equal(2, len(tasks))

	parser := shellwords.NewParser()
	parser.ParseEnv = true
	parser.ParseBacktick = true
	parser.Dir = repo

	var result string
	for _, task := range tasks {
		cmd, _ := task["cmd"]
		assert.NotEmpty(cmd)
		out, errCmd := carry(context.Background(), parser, repo, cmd)
		assert.NoError(errCmd)
		coverage, _ := task["coverage"]
		if coverage != "" {
			result, err = parseCoverage(coverage, out)
			assert.NoError(err)
		}
	}
	assert.Equal("42.9%", result)
}
