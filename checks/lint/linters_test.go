package lint

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/common"
)

func TestParseAPIDocCommands(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ref := common.GithubRef{}
	words, err := parseAPIDocCommands(ref, "../../testdata/go")
	require.NoError(err)
	assert.Equal(
		[]string{"apidoc", "-f", "file-filters", "-e", "exclude-filters", "-i", "input"},
		words,
	)
}

func TestOCLintResultXML(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	out, err := ioutil.ReadFile("../../testdata/oclint.xml")
	require.NoError(err)

	var violations OCLintResultXML
	err = xml.Unmarshal(out, &violations)
	assert.NoError(err)
	assert.NotEmpty(violations)
}

func TestKtlint(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	common.Conf.Core.Ktlint = "ktlint"
	ref := common.GithubRef{}
	lints, err := Ktlint(context.TODO(), ref, "example.kt", "../../testdata")
	require.NoError(err)
	require.Len(lints, 2)
	assert.Equal(1, lints[0].Line)
	assert.Equal(22, lints[1].Line)
}

func TestGolangCILint(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)
	currentDir := path.Dir(filename)

	common.Conf.Core.GolangCILint = "golangci-lint"

	projectPath := path.Join(currentDir, "../../testdata/go")
	ref := common.GithubRef{}
	result, msg, err := GolangCILint(context.TODO(), ref, projectPath)
	require.NoError(err)
	assert.Empty(msg)
	assert.NotNil(result)
	assert.NotEmpty(result.Issues)
	assert.Equal(3, result.Issues[0].Pos.Line)
}

func TestIsOC(t *testing.T) {
	assert.False(t, isOC("abc"))
	assert.True(t, isOC("abc.mm"))
}
