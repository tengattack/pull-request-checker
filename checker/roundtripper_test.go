package checker

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customRoundTripper struct {
}

func (*customRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestJWT(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	privateKey, err := ioutil.ReadFile("../config/test/sample_key.pem")
	require.NoError(err)
	var ts http.RoundTripper = newJWTRoundTripper(0, privateKey, &customRoundTripper{})

	req := httptest.NewRequest(http.MethodGet, "https://api.github.com/app", nil)
	ts.RoundTrip(req)
	assert.NotEmpty(req.Header.Get("Authorization"))
}
