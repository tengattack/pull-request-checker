package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveCommitsInfo(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	fileDB := "file name.db"
	require.NoError(Init(fileDB))

	c1 := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Author:   "author",
		Test:     "Go",
		Coverage: nil,
	}
	assert.NoError(c1.Save())

	c2 := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Author:   "author",
		Test:     "C",
		Coverage: nil,
	}
	assert.NoError(c2.Save())
	Deinit()

	// Init should be idempotent
	require.NoError(Init(fileDB))
	defer os.Remove(fileDB)
	defer Deinit()

	cc, err := ListCommitsInfo(c1.Owner, c1.Repo, c1.Sha)
	assert.NoError(err)
	assert.ElementsMatch(cc, []CommitsInfo{*c1, *c2})

	coverage := 0.5
	c1.Coverage = &coverage

	assert.NoError(c1.Save())

	c, err := LoadCommitsInfo(c1.Owner, c1.Repo, c1.Sha, c1.Test)
	assert.NoError(err)
	assert.Equal(c1, c)

	c, err = LoadCommitsInfo(c1.Owner, c1.Repo, c1.Sha, "php")
	assert.NoError(err)
	assert.Empty(c)

	cc, err = ListCommitsInfo(c1.Owner, c1.Repo, "Sha")
	assert.NoError(err)
	assert.Empty(c)
}
