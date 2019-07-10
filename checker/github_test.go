package checker

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/config"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

func TestMain(m *testing.M) {
	Conf = config.BuildDefaultConf()
	err := InitLog(Conf)
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
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
	appID := 35105
	installationID := 1248133 // tengattack/playground
	tr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport,
		appID, installationID, path.Join(currentDir, "../testdata/unified-ci-test.2019-07-09.private-key.pem"))
	require.NoError(err)

	client = github.NewClient(&http.Client{Transport: tr})

	testDiffFile := path.Join(currentDir, "../testdata/sillycode.cpp.diff")
	out, err := ioutil.ReadFile(testDiffFile)
	require.NoError(err)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	ctx := context.Background()
	ref := GithubRef{owner: "tengattack", repo: "playground"}

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
	_, _ = client.Issues.RemoveLabelsForIssue(ctx, ref.owner, ref.repo, 1)
}
