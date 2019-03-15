package checker

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

var jwtClient *github.Client

func InitJWTClient(id int, privateKeyFile string) error {
	privateKey, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return fmt.Errorf("could not read private key: %s", err)
	}
	tr := NewJWTRoundTripper(id, privateKey)
	jwtClient = github.NewClient(&http.Client{Transport: tr})
	return nil
}

type JWTRoundTripper struct {
	transport http.RoundTripper
	iss       int
	key       []byte

	mu  *sync.Mutex // mu protects token
	jwt *string
	exp time.Time
}

func NewJWTRoundTripper(iss int, key []byte) *JWTRoundTripper {
	return &JWTRoundTripper{
		iss:       iss,
		key:       key,
		transport: http.DefaultTransport,
		mu:        &sync.Mutex{},
	}
}

func (j *JWTRoundTripper) GetToken() (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.jwt == nil || j.exp.Add(-time.Minute).Before(time.Now()) {
		exp := time.Now().Add(10 * time.Minute)
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iat": int32(time.Now().Unix()),
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

func (j *JWTRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	token, err := j.GetToken()
	if err != nil {
		return nil, err
	}

	r.Header.Set("Authorization", "bearer "+token)
	return j.transport.RoundTrip(r)
}
