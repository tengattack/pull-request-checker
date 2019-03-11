package checker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/github"
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

func searchGithubPR(repo, sha string) (int64, error) {
	if sha == "" {
		return 0, errors.New("SHA is empty")
	}
	params := fmt.Sprintf("?q=is:pr+repo:%s+SHA:%s", repo, sha)

	req, err := http.NewRequest(http.MethodGet, GITHUB_API_URL+"/search/issues"+params, nil)
	if err != nil {
		return 0, err
	}
	var result struct {
		TotalCount        int64 `json:"total_count"`
		IncompleteResults bool  `json:"incomplete_results"`
		Items             []struct {
			Number int64 `json:"number"`
		} `json:"items"`
	}
	err = DoHTTPRequest(req, true, &result)
	if err != nil {
		return 0, err
	}
	if len(result.Items) == 0 {
		return 0, errors.New("PR number not found")
	}
	return result.Items[0].Number, nil
}
