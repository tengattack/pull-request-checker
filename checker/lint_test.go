package checker

import (
	"encoding/xml"
	"io/ioutil"
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
