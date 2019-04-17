package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveCommitsInfo(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	err := Init("file.db")
	require.NoError(err)
	defer Deinit()

	c := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Coverage: nil,
		Author:   "author",
	}

	assert.NoError(c.Save())

	cc, err := LoadCommitsInfo(c.Owner, c.Repo, c.Sha)
	assert.NoError(err)
	assert.Equal(cc, c)

	coverage := 0.5
	c.Coverage = &coverage

	assert.NoError(c.Save())

	cc, err = LoadCommitsInfo(c.Owner, c.Repo, c.Sha)
	assert.NoError(err)
	assert.Equal(cc, c)
}
