package common

import (
	"fmt"
	"io/ioutil"
	"math/rand"
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
var discoveryLock sync.RWMutex
var discoveryClients = make(map[string]*DiscoveryClient)

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

func getDiscoveryClient(appid string) *DiscoveryClient {
	discoveryLock.RLock()
	c, ok := discoveryClients[appid]
	discoveryLock.RUnlock()
	if !ok {
		discoveryLock.Lock()
		defer discoveryLock.Unlock()
		c, ok = discoveryClients[appid]
		if ok {
			return c
		}
		c = NewDiscoveryClient(appid)
		discoveryClients[appid] = c
	}
	return c
}

func getDiscoveryAddr(appid string, schemes []string) (*url.URL, error) {
	client := getDiscoveryClient(appid)
	instance, err := client.Instance()
	if err != nil {
		return nil, err
	}
	var urls []*url.URL
	for _, addr := range instance.Addrs {
		u, err := url.Parse(addr)
		if err != nil {
			continue
		}
		for _, scheme := range schemes {
			if u.Scheme == scheme {
				urls = append(urls, u)
			}
		}
	}
	if len(urls) == 0 {
		return nil, errors.New("no available discovery addr")
	}
	return urls[rand.Intn(len(urls))], nil
}

func ProxyURL() (*url.URL, error) {
	if Conf.Core.Socks5Proxy != "" {
		u, err := url.Parse(Conf.Core.Socks5Proxy)
		if err != nil {
			return nil, fmt.Errorf("Setup proxy failed: %v", err)
		}
		if u.Scheme == "discovery" {
			u, err = getDiscoveryAddr(u.Host, []string{"socks5"})
			if err != nil {
				return nil, fmt.Errorf("Setup proxy failed: %v", err)
			}
		}
		if u.Scheme != "socks5" {
			return nil, fmt.Errorf("Setup proxy failed: unknown socks5 proxy url")
		}
		return u, nil
	}

	if Conf.Core.HTTPProxy != "" {
		u, err := url.Parse(Conf.Core.HTTPProxy)
		if err != nil {
			return nil, fmt.Errorf("Setup proxy failed: %v", err)
		}
		if u.Scheme == "discovery" {
			u, err = getDiscoveryAddr(u.Host, []string{"http", "https"})
			if err != nil {
				return nil, fmt.Errorf("Setup proxy failed: %v", err)
			}
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("Setup proxy failed: unknown http proxy url")
		}
		return u, nil
	}

	return nil, nil
}

func newProxyRoundTripper() (http.RoundTripper, error) {
	var tr http.RoundTripper

	u, err := ProxyURL()
	if err != nil {
		return nil, err
	}

	if u == nil {
		tr = http.DefaultTransport
	} else if u.Scheme == "socks5" {
		dialSocksProxy, err := proxy.SOCKS5("tcp", Conf.Core.Socks5Proxy, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("Setup proxy failed: %v", err)
		}
		tr = &http.Transport{Dial: dialSocksProxy.Dial}
	} else {
		tr = &http.Transport{Proxy: http.ProxyURL(u)}
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
