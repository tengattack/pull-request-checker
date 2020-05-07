package checker

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAPIDocCommands(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ref := GithubRef{}
	words, err := parseAPIDocCommands(ref, "../testdata/go")
	require.NoError(err)
	assert.Equal(
		[]string{"apidoc", "-f", "file-filters", "-e", "exclude-filters", "-i", "input"},
		words,
	)
}

func TestOCLintResultXML(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	out, err := ioutil.ReadFile("../testdata/oclint.xml")
	require.NoError(err)

	var violations OCLintResultXML
	err = xml.Unmarshal(out, &violations)
	assert.NoError(err)
	assert.NotEmpty(violations)
}

func TestKtlint(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	Conf.Core.Ktlint = "ktlint"
	ref := GithubRef{}
	lints, err := Ktlint(context.TODO(), ref, "example.kt", "../testdata")
	require.NoError(err)
	assert.NotEmpty(lints)
}

func TestGolangCILint(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)
	currentDir := path.Dir(filename)

	Conf.Core.GolangCILint = "golangci-lint"

	projectPath := path.Join(currentDir, "../testdata/go")
	ref := GithubRef{}
	result, msg, err := GolangCILint(context.TODO(), ref, projectPath)
	require.NoError(err)
	assert.Empty(msg)
	assert.NotNil(result)
	assert.NotEmpty(result.Issues)
	assert.Equal("deadcode", result.Issues[0].FromLinter)
}

func TestCheckFileMode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	repoPath := "../testdata/src"
	fileName := "src.diff"
	out, err := ioutil.ReadFile(path.Join(repoPath, fileName))
	require.NoError(err)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	var buf strings.Builder
	lints, problems, err := CheckFileMode(diffs, repoPath, &buf)
	assert.NoError(err)
	assert.Equal(4, problems)
	for _, v := range lints {
		switch v.GetPath() {
		case "c.sh":
			assert.Equal(fileModeCheckShellScript, v.GetMessage())
		case "d.sh":
			assert.Equal(shebangCheckShellScript, v.GetMessage())
		case "h.py":
			assert.Equal(fileModeCheckExecutable, v.GetMessage())
		case "b.py":
			assert.Equal(fileModeCheckNormal, v.GetMessage())
		default:
			assert.Fail(v.GetPath() + " should not be ok")
		}
	}
}
