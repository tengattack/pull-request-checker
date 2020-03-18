package checker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar"
	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/pkg/errors"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/tengattack/unified-ci/util"
	yaml "gopkg.in/yaml.v2"
)

const (
	projectTestsConfigFile = ".unified-ci.yml"
)

var (
	percentageRegexp = regexp.MustCompile(`[-+]?(?:\d*\.\d+|\d+)%`)
)

type panicError struct {
	Info interface{}
}

func (p *panicError) Error() (s string) {
	if p != nil {
		s = fmt.Sprintf("Panic: %v", p.Info)
	}
	return
}

// InitHTTPRequest helps to set necessary headers
func InitHTTPRequest(req *http.Request, isJSONResponse bool) {
	if isJSONResponse {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", UserAgent())
}

// DoHTTPRequest sends request and gets response to struct
func DoHTTPRequest(req *http.Request, isJSONResponse bool, v interface{}) error {
	InitHTTPRequest(req, isJSONResponse)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	// close response
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	LogAccess.Debugf("HTTP %s\n%s", resp.Status, body)

	if isJSONResponse {
		err = json.Unmarshal(body, &v)
		if err != nil && resp.StatusCode != 200 {
			return errors.New("HTTP " + resp.Status)
		}
	} else {
		if ret, ok := v.(*[]byte); ok {
			*ret = body
		}
	}

	return err
}

// UpdateCheckRunWithError updates the check run result with error message
func UpdateCheckRunWithError(ctx context.Context, client *github.Client, gpull *github.PullRequest, checkRunID int64, checkName, outputTitle string, err error) {
	if gpull != nil {
		conclusion := "action_required"
		checkRunStatus := "completed"
		t := github.Timestamp{Time: time.Now()}
		outputSummary := fmt.Sprintf("error: %v", err)

		owner := gpull.GetBase().GetRepo().GetOwner().GetLogin()
		repo := gpull.GetBase().GetRepo().GetName()
		_, _, erro := client.Checks.UpdateCheckRun(ctx, owner, repo, checkRunID, github.UpdateCheckRunOptions{
			Name:        checkName,
			Status:      &checkRunStatus,
			Conclusion:  &conclusion,
			CompletedAt: &t,
			Output: &github.CheckRunOutput{
				Title:   &outputTitle,
				Summary: &outputSummary,
			},
		})
		if erro != nil {
			LogError.Errorf("github update check run with error failed: %v", erro)
		}
	}
}

// UpdateCheckRun updates the check run result with output message
// outputTitle, outputSummary can contain markdown.
func UpdateCheckRun(ctx context.Context, client *github.Client, gpull *github.PullRequest, checkRunID int64, checkName string, conclusion string, t github.Timestamp, outputTitle string, outputSummary string, annotations []*github.CheckRunAnnotation) error {
	checkRunStatus := "completed"
	// Only 65535 characters are allowed in this request
	if len(outputSummary) > 60000 {
		_, outputSummary = util.Truncated(outputSummary, "... truncated ...", 60000)
		LogError.Warn("The output summary is too long.")
	}
	owner := gpull.GetBase().GetRepo().GetOwner().GetLogin()
	repo := gpull.GetBase().GetRepo().GetName()
	_, _, err := client.Checks.UpdateCheckRun(ctx, owner, repo, checkRunID, github.UpdateCheckRunOptions{
		Name:        checkName,
		Status:      &checkRunStatus,
		Conclusion:  &conclusion,
		CompletedAt: &t,
		Output: &github.CheckRunOutput{
			Title:       &outputTitle,
			Summary:     &outputSummary,
			Annotations: annotations,
		},
	})
	if err != nil {
		LogError.Errorf("github update check run failed: %v", err)
	}
	return err
}

// CreateCheckRun creates a new check run
func CreateCheckRun(ctx context.Context, client *github.Client, gpull *github.PullRequest, checkName string, ref GithubRef, targetURL string) (*github.CheckRun, error) {
	checkRunStatus := "in_progress"

	owner := gpull.GetBase().GetRepo().GetOwner().GetLogin()
	repo := gpull.GetBase().GetRepo().GetName()
	checkRun, _, err := client.Checks.CreateCheckRun(ctx, owner, repo, github.CreateCheckRunOptions{
		Name:       checkName,
		HeadSHA:    ref.Sha,
		DetailsURL: &targetURL,
		Status:     &checkRunStatus,
	})
	return checkRun, err
}

type goTestsConfig struct {
	Coverage string   `yaml:"coverage"`
	Cmds     []string `yaml:"cmds"`
}

type projectConfig struct {
	LinterAfterTests bool                     `yaml:"linterAfterTests"`
	Tests            map[string]goTestsConfig `yaml:"tests"`
	IgnorePatterns   []string                 `yaml:"ignorePatterns"`
}

type projectConfigRaw struct {
	LinterAfterTests bool                `yaml:"linterAfterTests"`
	Tests            map[string][]string `yaml:"tests"`
	IgnorePatterns   []string            `yaml:"ignorePatterns"`
}

func isEmptyTest(cmds []string) bool {
	empty := true
	for _, c := range cmds {
		if c != "" {
			empty = false
		}
	}
	return empty
}

func readProjectConfig(cwd string) (config projectConfig, err error) {
	content, err := ioutil.ReadFile(filepath.Join(cwd, projectTestsConfigFile))
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return config, err
	}

	err = yaml.Unmarshal(content, &config)
	if err != nil {
		var cfg projectConfigRaw
		err = yaml.Unmarshal(content, &cfg)
		if err != nil {
			return config, err
		}
		config.Tests = make(map[string]goTestsConfig)
		for k, v := range cfg.Tests {
			config.Tests[k] = goTestsConfig{Cmds: v, Coverage: ""}
		}
	}
	return config, nil
}

func getDefaultAPIClient(owner string) (*github.Client, error) {
	var client *github.Client
	installationID, ok := Conf.GitHub.Installations[owner]
	if ok {
		tr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport,
			Conf.GitHub.AppID, installationID, Conf.GitHub.PrivateKey)
		if err != nil {
			return nil, err
		}

		client = github.NewClient(&http.Client{Transport: tr})
		return client, nil
	}
	return nil, errors.New("InstallationID not found, owner: " + owner)
}

// NewShellParser returns a shell parser
func NewShellParser(repoPath string, ref GithubRef) *shellwords.Parser {
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
			return ref.checkType
		case "CI_CHECK_REF":
			return ref.checkRef
		}
		return os.Getenv(key)
	}

	return parser
}

// MatchAny checks if path matches any of the given patterns
func MatchAny(patterns []string, path string) bool {
	for _, pattern := range patterns {
		match, _ := doublestar.Match(pattern, path)
		if match {
			return true
		}
	}
	return false
}

// FibonacciBinet calculates fibonacci value by analytic (Binet's formula)
func FibonacciBinet(num int64) int64 {
	n := float64(num)
	return int64(((math.Pow(((1+math.Sqrt(5))/2), n) - math.Pow(1-((1+math.Sqrt(5))/2), n)) / math.Sqrt(5)) + 0.5)
}

func getTrimmedNewName(d *diff.FileDiff) (string, bool) {
	newName := util.Unquote(d.NewName)
	if strings.HasPrefix(newName, "b/") {
		return newName[2:], true
	}
	return newName, false
}

func headFile(file string, n int) (lines []string, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	for i := 1; s.Scan() && i <= n; i++ {
		lines = append(lines, s.Text())
	}
	return lines, s.Err()
}
