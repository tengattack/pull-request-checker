package common

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"golang.org/x/net/proxy"
)

// JWTClient is used for JWT authorization
var JWTClient *github.Client

// InitJWTClient initializes the jwtClient
func InitJWTClient(id int64, privateKeyFile string) error {
	privateKey, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return fmt.Errorf("could not read private key: %s", err)
	}
	tr, err := newProxyRoundTripper()
	if err != nil {
		return err
	}
	tr = newJWTRoundTripper(id, privateKey, tr)
	JWTClient = github.NewClient(&http.Client{Transport: tr})
	return nil
}

func newProxyRoundTripper() (http.RoundTripper, error) {
	var tr http.RoundTripper
	if Conf.Core.Socks5Proxy != "" {
		dialSocksProxy, err := proxy.SOCKS5("tcp", Conf.Core.Socks5Proxy, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("Setup proxy failed: %v", err)
		}
		tr = &http.Transport{Dial: dialSocksProxy.Dial}
	} else if Conf.Core.HTTPProxy != "" {
		proxy, err := url.Parse(Conf.Core.HTTPProxy)
		if err != nil {
			return nil, fmt.Errorf("Setup proxy failed: %v", err)
		}
		if proxy.Scheme != "http" && proxy.Scheme != "https" {
			return nil, fmt.Errorf("Setup proxy failed: unknown http proxy url")
		}
		tr = &http.Transport{Proxy: http.ProxyURL(proxy)}
	} else {
		tr = http.DefaultTransport
	}
	return tr, nil
}

type jwtRoundTripper struct {
	transport http.RoundTripper
	iss       int64
	key       []byte

	mu  *sync.Mutex // mu protects token
	jwt *string
	exp time.Time
}

func newJWTRoundTripper(iss int64, key []byte, transport http.RoundTripper) *jwtRoundTripper {
	return &jwtRoundTripper{
		iss:       iss,
		key:       key,
		transport: transport,
		mu:        &sync.Mutex{},
	}
}

func (j *jwtRoundTripper) GetToken() (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.jwt == nil || j.exp.Add(-time.Minute).Before(time.Now()) {
		now := time.Now()
		exp := now.Add(10 * time.Minute)
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iat": int32(now.Unix()),
			"exp": int32(exp.Unix()),
			"iss": j.iss,
		})

		signKey, err := jwt.ParseRSAPrivateKeyFromPEM(j.key)
		if err != nil {
			return "", errors.Wrap(err, "failed to parse key")
		}

		tokenString, err := token.SignedString(signKey)
		if err != nil {
			return "", errors.Wrap(err, "failed to sign token")
		}
		j.jwt = &tokenString
		j.exp = exp
	}

	return *j.jwt, nil
}

func (j *jwtRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	token, err := j.GetToken()
	if err != nil {
		return nil, err
	}

	r.Header.Set("Authorization", "Bearer "+token)
	return j.transport.RoundTrip(r)
}
