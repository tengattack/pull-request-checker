package util

import (
	"context"
	"regexp"
	"sync"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchGithubPR(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	client := github.NewClient(nil)
	i, err := SearchGithubPR(context.Background(), client, "tengattack/unified-ci", "7988bac704d600a86bd29149c569c788f0d7cd92")
	require.NoError(err)
	assert.EqualValues(23, i)
}

func TestDiffCoverage(t *testing.T) {
	assert := assert.New(t)

	var head, base sync.Map

	base.Store("down", "60%")
	head.Store("down", "50%")

	base.Store("up", "50%")
	head.Store("up", "60%")

	base.Store("na", "unknown")
	head.Store("na", "50%")

	output := DiffCoverage(&head, &base)
	assert.Regexp(regexp.MustCompile("(?s)```diff\n.*\n```"), output)
	assert.Regexp(regexp.MustCompile(`(?m)^\+.*up.*50%.*60%`), output)
	assert.Regexp(regexp.MustCompile(`(?m)^-.*down.*60%.*50%`), output)
	assert.Regexp(regexp.MustCompile(`(?m)^ .*na.*unknown.*50%`), output)
}

func TestGetBaseSHA(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	client := github.NewClient(nil)
	sha, err := GetBaseSHA(client, "tengattack", "unified-ci", 30)
	require.NoError(err)
	assert.Equal("94a32a63aa2a618a127a00954bb9965bff8939df", sha)
}
