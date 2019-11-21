package checker

import (
	"context"
	"path"
	"runtime"
	"testing"

	shellwords "github.com/mattn/go-shellwords"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverRegex(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filepath, _, _ := runtime.Caller(0)
	curDir := path.Dir(filepath)
	repo := curDir + "/../testdata/go"

	repoConf, err := readProjectConfig(repo)
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
	for _, cmd := range test.Cmds {
		out, errCmd := carry(context.Background(), parser, repo, cmd)
		assert.NoError(errCmd)
		output += ("\n" + out)
	}

	if test.Coverage != "" {
		result, pct, err = parseCoverage(test.Coverage, output)
		assert.NoError(err)
	}
	assert.Regexp(percentageRegexp, result)
	assert.True(pct > 0)
}
