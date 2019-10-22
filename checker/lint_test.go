package checker_test

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/checker"
)

func TestAPIDoc(t *testing.T) {
	assert := assert.New(t)

	output, err := checker.APIDoc(context.Background(), "../testdata/go")
	assert.Error(err)
	assert.NotEmpty(output)
}

func TestOCLintResultXML(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	out, err := ioutil.ReadFile("../testdata/oclint.xml")
	require.NoError(err)

	var violations checker.OCLintResultXML
	err = xml.Unmarshal(out, &violations)
	assert.NoError(err)
	assert.NotEmpty(violations)
}
