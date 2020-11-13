package worker

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/store"
)

func TestBadgesHandler(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	resp := httptest.NewRecorder()
	c, r := gin.CreateTestContext(resp)

	r.GET("/badges/:owner/:repo/:type", BadgesHandler)

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
