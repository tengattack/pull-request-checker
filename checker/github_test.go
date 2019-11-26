package checker

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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

func TestBadgesHandler(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	resp := httptest.NewRecorder()
	c, r := gin.CreateTestContext(resp)

	r.GET("/badges/:owner/:repo/:type", badgesHandler)

	// bad request
	resp = httptest.NewRecorder()
	c.Request = httptest.NewRequest(http.MethodGet, "/badges/Test/NonExists/unknown.svg", nil)
	r.ServeHTTP(resp, c.Request)
	assert.Equal(http.StatusBadRequest, resp.Code)

	// non exists
	resp = httptest.NewRecorder()
	c.Request = httptest.NewRequest(http.MethodGet, "/badges/Test/NonExists/build.svg", nil)
	r.ServeHTTP(resp, c.Request)
	assert.Equal(http.StatusOK, resp.Code)
	assert.Contains(resp.Body.String(), ">build<")
	assert.Contains(resp.Body.String(), ">unknown<")

	coverage := float64(0.201)
	commitInfo := store.CommitsInfo{
		Owner:    "Test",
		Repo:     "Exists",
		Sha:      "sha",
		Author:   "nobody",
		Test:     "test",
		Coverage: &coverage,
		Passing:  0,
		Status:   0, // unmerged to master
	}
	require.NoError(commitInfo.Save())

	resp = httptest.NewRecorder()
	c.Request = httptest.NewRequest(http.MethodGet, "/badges/Test/Exists/coverage.svg", nil)
	r.ServeHTTP(resp, c.Request)
	assert.Equal(http.StatusOK, resp.Code)
	assert.Contains(resp.Body.String(), ">coverage<")
	assert.Contains(resp.Body.String(), ">unknown<")

	commitInfo.Status = 1 // merged to master
	require.NoError(commitInfo.Save())

	resp = httptest.NewRecorder()
	c.Request = httptest.NewRequest(http.MethodGet, "/badges/Test/Exists/coverage.svg", nil)
	r.ServeHTTP(resp, c.Request)
	assert.Equal(http.StatusOK, resp.Code)
	assert.Contains(resp.Body.String(), ">coverage<")
	assert.Contains(resp.Body.String(), ">20%<") // round to integer

	resp = httptest.NewRecorder()
	c.Request = httptest.NewRequest(http.MethodGet, "/badges/Test/Exists/build.svg", nil)
	r.ServeHTTP(resp, c.Request)
	assert.Equal(http.StatusOK, resp.Code)
	assert.Contains(resp.Body.String(), ">build<")
	assert.Contains(resp.Body.String(), ">failing<") // round to integer

	commitInfo.Passing = 1 // passing
	require.NoError(commitInfo.Save())

	resp = httptest.NewRecorder()
	c.Request = httptest.NewRequest(http.MethodGet, "/badges/Test/Exists/build.svg", nil)
	r.ServeHTTP(resp, c.Request)
	assert.Equal(http.StatusOK, resp.Code)
	assert.Contains(resp.Body.String(), ">build<")
	assert.Contains(resp.Body.String(), ">passing<") // round to integer
}
