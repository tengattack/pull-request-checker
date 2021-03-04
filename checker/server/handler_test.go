package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/common"
)

func newReq(uri, body string) (resp *httptest.ResponseRecorder) {
	resp = httptest.NewRecorder()
	c, r := gin.CreateTestContext(resp)

	// routes
	r.POST("/api/queue/add", addQueueHandler)

	c.Request = httptest.NewRequest(http.MethodPost, uri, strings.NewReader(body))
	c.Request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	r.ServeHTTP(resp, c.Request)
	return
}

func TestAddQueueHandler(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NotPanics(func() { common.MQ.Reset() })

	uri := "/api/queue/add"

	form := url.Values{}

	// bad request
	resp := newReq(uri, form.Encode())
	assert.Equal(http.StatusBadRequest, resp.Code)

	// bad request (message)
	form.Set("message", "tengattack/playground/pull/2")
	resp = newReq(uri, form.Encode())
	assert.Equal(http.StatusBadRequest, resp.Code)

	// direct message
	form.Set("message", "tengattack/playground/pull/2/commits/ae26afcc1d5c268ba751a5903828e0423bd87cf2")
	resp = newReq(uri, form.Encode())
	assert.Equal(http.StatusOK, resp.Code)

	// bad request (url)
	form = url.Values{}
	form.Set("url", "https://github.com/tengattack/playground/pull")
	resp = newReq(uri, form.Encode())
	assert.Equal(http.StatusBadRequest, resp.Code)

	// repo url
	form = url.Values{}
	form.Set("url", "https://github.com/tengattack/playground")
	resp = newReq(uri, form.Encode())
	assert.Equal(http.StatusOK, resp.Code)

	// repo PR url
	form = url.Values{}
	form.Set("url", "https://github.com/tengattack/playground/pull/2")
	resp = newReq(uri, form.Encode())
	assert.Equal(http.StatusOK, resp.Code)

	// repo branch url
	form = url.Values{}
	form.Set("url", "https://github.com/tengattack/playground/tree/master")
	resp = newReq(uri, form.Encode())
	assert.Equal(http.StatusOK, resp.Code)

	// direct url message
	form = url.Values{}
	form.Set("url", "https://github.com/tengattack/playground/pull/2/commits/83295acbd3f4a676931c233735157b5470cef5aa")
	resp = newReq(uri, form.Encode())
	assert.Equal(http.StatusOK, resp.Code)
}
