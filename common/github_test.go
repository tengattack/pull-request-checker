package common

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/gin-gonic/gin"
	"github.com/google/go-github/github"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/store"
)

func TestMain(m *testing.M) {
	Conf = config.BuildDefaultConf()
	err := InitLog(Conf)
	if err != nil {
		panic(err)
	}

	fileDB := "file name.db"
	err = store.Init(fileDB)
	if err != nil {
		panic(err)
	}

	gin.SetMode(gin.TestMode)

	code := m.Run()

	// clean up
	store.Deinit()
	os.Remove(fileDB)

	os.Exit(code)
}

func TestLabelPRSize(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)
	currentDir := path.Dir(filename)

	var client *github.Client

	// Wrap the shared transport for use with the integrati
	// TODO: add installation ID to db
	appID := int64(35105)
	installationID := int64(1248133) // tengattack/playground
	tr, err := newProxyRoundTripper()
	require.NoError(err)
	tr, err = ghinstallation.NewKeyFromFile(tr,
		appID, installationID, path.Join(currentDir, "../testdata/unified-ci-test.2020-10-11.private-key.pem"))
	require.NoError(err)

	client = github.NewClient(&http.Client{Transport: tr})

	testDiffFile := path.Join(currentDir, "../testdata/sillycode.cpp.diff")
	out, err := ioutil.ReadFile(testDiffFile)
	require.NoError(err)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	ctx := context.Background()
	ref := GithubRef{Owner: "tengattack", RepoName: "playground"}

	err = LabelPRSize(ctx, client, ref, 1, diffs)
	assert.NoError(err)

	testDiffFile = path.Join(currentDir, "../testdata/test1.go.diff")
	out, err = ioutil.ReadFile(testDiffFile)
	require.NoError(err)

	diffs, err = diff.ParseMultiFileDiff(out)
	require.NoError(err)
	err = LabelPRSize(ctx, client, ref, 1, diffs)
	assert.NoError(err)

	// TODO: check more conditions

	// cleanup
	_, _ = client.Issues.RemoveLabelsForIssue(ctx, ref.Owner, ref.RepoName, 1)
}

func TestSearchGithubPR(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	client := github.NewClient(nil)
	i, err := SearchGithubPR(context.Background(), client, "tengattack/unified-ci", "7988bac704d600a86bd29149c569c788f0d7cd92")
	require.NoError(err)
	assert.EqualValues(23, i)
}
