package checker

import (
	"encoding/xml"
	"io/ioutil"
	"strings"
	"testing"

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

func TestCheckFileMode(t *testing.T) {
	assert := assert.New(t)

	var buf strings.Builder
	msg, problems, err := CheckFileMode(&buf, "../testdata/src")
	assert.NoError(err)
	assert.Equal(problems, 0)
	assert.Empty(msg)
}
