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
	err := Init(fileDB)
	require.NoError(err)

	c := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Author:   "author",
		Coverage: nil,
	}
	assert.NoError(c.Save())
	Deinit()

	// Init should be idempotent
	err = Init(fileDB)
	require.NoError(err)
	defer os.Remove(fileDB)
	defer Deinit()

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
