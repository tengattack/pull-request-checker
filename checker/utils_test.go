package checker

import (
	"path"
	"runtime"
	"testing"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchAny(t *testing.T) {
	assert := assert.New(t)

	assert.True(MatchAny([]string{"sdk/**"}, "sdk/v2/x"))
	assert.False(MatchAny([]string{"sdk/*"}, "sdk/v2/x"))
}

func TestReadProjectConfig(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	repoConf, err := readProjectConfig(currentDir)
	require.NoError(err)
	assert.Empty(repoConf.Tests)

	repoConf, err = readProjectConfig(currentDir + "/../")
	require.NoError(err)
	assert.True(len(repoConf.Tests) > 0)
	assert.Equal([]string{
		"testdata/**",
		"sdk/**",
	}, repoConf.IgnorePatterns)
}

func TestNewShellParser(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	ref := GithubRef{
		checkType: CheckTypeBranch,
		checkRef:  "stable",
	}
	parser := NewShellParser(currentDir, ref)
	require.NotNil(parser)

	words, err := parser.Parse("echo $PWD $PROJECT_NAME $CI_CHECK_TYPE $CI_CHECK_REF")
	require.NoError(err)
	assert.Equal([]string{"echo", currentDir, "checker", CheckTypeBranch, "stable"}, words)
}

func TestFibonacciBinet(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(int64(1), FibonacciBinet(1))
	assert.Equal(int64(1), FibonacciBinet(2))
	assert.Equal(int64(5), FibonacciBinet(5))
	assert.Equal(int64(55), FibonacciBinet(10))
	assert.Equal(int64(6765), FibonacciBinet(20))
}

func TestGetTrimmedNewName(t *testing.T) {
	assert := assert.New(t)

	name, ok := getTrimmedNewName(&diff.FileDiff{NewName: "b/name"})
	assert.True(ok)
	assert.Equal("name", name)

	name, ok = getTrimmedNewName(&diff.FileDiff{NewName: "b/hello \342\230\272.md"})
	assert.True(ok)
	assert.Equal("hello ☺.md", name)

	name, ok = getTrimmedNewName(&diff.FileDiff{NewName: "name"})
	assert.False(ok)
	assert.Equal("name", name)
}
