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
	defer os.Remove(fileDB)
	defer Deinit()

	c1 := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Author:   "author",
		Test:     "Go",
		Coverage: nil,
	}
	assert.NoError(c1.Save())
	assert.NotEmpty(c1.CreateTime)

	info, err := GetLatestCommitsInfo(c1.Owner, c1.Repo)
	assert.NoError(err)
	assert.Nil(info)

	c2 := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Author:   "author",
		Test:     "C",
		Coverage: nil,
		Passing:  0,
		Status:   1,
	}
	assert.NoError(c2.Save())
	assert.NotEmpty(c2.CreateTime)

	info, err = GetLatestCommitsInfo(c2.Owner, c2.Repo)
	assert.NoError(err)
	assert.Len(info, 1)
	assert.Equal("C", info[0].Test)

	coverage := float64(-1)
	c2.Coverage = &coverage
	assert.NoError(c2.Save())

	info, err = GetLatestCommitsInfo(c2.Owner, c2.Repo)
	assert.NoError(err)
	assert.Nil(info)

	coverage = 0.5
	c1.Coverage = &coverage
	c1.Status = 1
	assert.NoError(c1.Save())

	info, err = GetLatestCommitsInfo(c2.Owner, c2.Repo)
	assert.NoError(err)
	assert.Len(info, 1)
	assert.Equal("Go", info[0].Test)
}

func TestListCommitsInfo(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	fileDB := "file name.db"
	require.NoError(Init(fileDB))
	defer os.Remove(fileDB)
	defer Deinit()

	c1 := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Author:   "author",
		Test:     "Go",
		Coverage: nil,
	}
	assert.NoError(c1.Save())
	assert.NotEmpty(c1.CreateTime)

	c2 := &CommitsInfo{
		Owner:    "owner",
		Repo:     "repo",
		Sha:      "sha",
		Author:   "author",
		Test:     "C",
		Coverage: nil,
		Passing:  0,
		Status:   1,
	}
	assert.NoError(c2.Save())
	assert.NotEmpty(c2.CreateTime)

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
