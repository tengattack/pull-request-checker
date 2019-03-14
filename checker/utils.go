package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"sourcegraph.com/sourcegraph/go-diff/diff"
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
	req.Header.Set("User-Agent", UserAgent)
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
func UpdateCheckRunWithError(ctx context.Context, client *github.Client, gpull *GithubPull, checkRunID int64, checkName, outputTitle string, err error) {
	if gpull != nil {
		conclusion := "action_required"
		checkRunStatus := "completed"
		t := github.Timestamp{Time: time.Now()}
		outputSummary := fmt.Sprintf("error: %v", err)
		_, _, eror := client.Checks.UpdateCheckRun(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name, checkRunID, github.UpdateCheckRunOptions{
			Name:        checkName,
			Status:      &checkRunStatus,
			Conclusion:  &conclusion,
			CompletedAt: &t,
			Output: &github.CheckRunOutput{
				Title:   &outputTitle,
				Summary: &outputSummary,
			},
		})
		if eror != nil {
			LogError.Errorf("github update check run with error failed: %v", eror)
		}
	}
}

// UpdateCheckRun updates the check run result with output message
// outputTitle, outputSummary can contain markdown.
func UpdateCheckRun(ctx context.Context, client *github.Client, gpull *GithubPull, checkRunID int64, checkName string, conclusion string, t github.Timestamp, outputTitle string, outputSummary string, annotations []*github.CheckRunAnnotation) error {
	checkRunStatus := "completed"
	_, _, err := client.Checks.UpdateCheckRun(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name, checkRunID, github.UpdateCheckRunOptions{
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
func CreateCheckRun(ctx context.Context, client *github.Client, gpull *GithubPull, checkName string, ref GithubRef, targetURL string) (*github.CheckRun, error) {
	checkRunStatus := "in_progress"
	checkRun, _, err := client.Checks.CreateCheckRun(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name, github.CreateCheckRunOptions{
		Name:       checkName,
		HeadBranch: gpull.Base.Ref,
		HeadSHA:    ref.Sha,
		DetailsURL: &targetURL,
		Status:     &checkRunStatus,
	})
	return checkRun, err
}

func getTests(diffs []*diff.FileDiff) map[string]bool {
	result := make(map[string]bool)
	for _, d := range diffs {
		newName, err := strconv.Unquote(d.NewName)
		if err != nil {
			newName = d.NewName
		}
		if strings.HasPrefix(newName, "b/") {
			fileName := newName[2:]
			if strings.HasSuffix(fileName, ".go") {
				result["go"] = true
			} else if strings.HasSuffix(fileName, ".php") {
				result["php"] = true
			}
		}
	}
	return result
}

func searchGithubPR(ctx context.Context, client *github.Client, repo, sha string) (int, error) {
	if sha == "" {
		return 0, errors.New("SHA is empty")
	}
	q := fmt.Sprintf("is:pr repo:%s SHA:%s", repo, sha)
	opts := &github.SearchOptions{Sort: "created", Order: "asc"}
	result, _, err := client.Search.Issues(ctx, q, opts)
	if err != nil {
		return 0, err
	}
	if result.GetTotal() == 0 {
		return 0, errors.New("PR number not found")
	}
	return result.Issues[0].GetNumber(), nil
}

func getDefaultAPIClient(owner string, appID int, privateKeyPath string) (*github.Client, error) {
	var client *github.Client
	installationID, ok := Conf.GitHub.Installations[owner]
	if ok {
		tr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport,
			appID, installationID, privateKeyPath)
		if err != nil {
			return nil, err
		}

		client = github.NewClient(&http.Client{Transport: tr})
		return client, nil
	}
	return nil, errors.New("InstallationID not found, owner: " + owner)
}
