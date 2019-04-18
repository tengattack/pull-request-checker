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

	erro := os.Remove("file.db")
	require.NoError(erro)
	err := Init("file name.db")
	require.NoError(err)
	Deinit()

	err = Init("file name.db")
	require.NoError(err)
	defer os.Remove("file name.db")
	defer Deinit()

	c := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Author:   "author",
		Coverage: nil,
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
