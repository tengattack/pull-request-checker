module github.com/tengattack/unified-ci

go 1.13

replace github.com/google/go-github => github.com/google/go-github/v28 v28.0.0

require (
	github.com/bmatcuk/doublestar v1.2.2
	github.com/bradleyfalzon/ghinstallation v1.1.0
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/gin-gonic/gin v1.5.0
	github.com/google/go-github v17.0.0+incompatible
	github.com/jmoiron/sqlx v1.2.0
	github.com/martinlindhe/go-difflib v1.0.0
	github.com/mattn/go-isatty v0.0.11
	github.com/mattn/go-shellwords v1.0.6
	github.com/mattn/go-sqlite3 v2.0.2+incompatible
	github.com/pkg/errors v0.8.1
	github.com/sirupsen/logrus v1.4.2
	github.com/sourcegraph/go-diff v0.5.1
	github.com/sqs/goreturns v0.0.0-20181028201513-538ac6014518
	github.com/stretchr/testify v1.4.0
	github.com/thoas/stats v0.0.0-20190407194641-965cb2de1678
	golang.org/x/lint v0.0.0-20191125180803-fdd1cda4f05f
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/tools v0.0.0-20200109174759-ac4f524c1612
	gopkg.in/appleboy/gin-status-api.v1 v1.0.1
	gopkg.in/fukata/golang-stats-api-handler.v1 v1.0.0 // indirect
	gopkg.in/redis.v5 v5.2.9
	gopkg.in/rjz/githubhook.v0 v0.0.1
	gopkg.in/yaml.v2 v2.2.7
)
