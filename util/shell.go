package util

import (
	"os"
	"path/filepath"

	shellwords "github.com/mattn/go-shellwords"
	"github.com/tengattack/unified-ci/common"
)

// NewShellParser returns a shell parser
func NewShellParser(repoPath string, ref common.GithubRef) *shellwords.Parser {
	parser := shellwords.NewParser()
	parser.ParseEnv = true
	parser.ParseBacktick = true
	parser.Dir = repoPath

	projectName := filepath.Base(repoPath)

	parser.Getenv = func(key string) string {
		switch key {
		case "PWD":
			return repoPath
		case "PROJECT_NAME":
			return projectName
		case "CI_CHECK_TYPE":
			return ref.CheckType
		case "CI_CHECK_REF":
			return ref.CheckRef
		}
		return os.Getenv(key)
	}

	return parser
}
