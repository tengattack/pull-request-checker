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
	test, ok := tests["go"]
	require.True(ok)

	parser := shellwords.NewParser()
	parser.ParseEnv = true
	parser.ParseBacktick = true
	parser.Dir = repo

	var result string
	var output string
	for _, cmd := range test.Cmds {
		out, errCmd := carry(context.Background(), parser, repo, cmd)
		assert.NoError(errCmd)
		output += ("\n" + out)
	}

	if test.Coverage != "" {
		result, _, err = parseCoverage(test.Coverage, output)
		assert.NoError(err)
	}
	assert.Regexp(percentageRegexp, result)
}
