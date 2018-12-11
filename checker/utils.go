package checker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/go-github/github"
)

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
func UpdateCheckRunWithError(ctx context.Context, err error, client *github.Client, gpull *GithubPull, checkRunID int64) {
	if err != nil && checkRunID != 0 && gpull != nil {
		conclusion := "action_required"
		checkRunStatus := "completed"
		t := github.Timestamp{Time: time.Now()}
		outputTitle := "linter"
		outputSummary := fmt.Sprintf("error: %v", err)
		_, _, eror := client.Checks.UpdateCheckRun(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name, checkRunID, github.UpdateCheckRunOptions{
			Name:        "linter",
			Status:      &checkRunStatus,
			Conclusion:  &conclusion,
			CompletedAt: &t,
			Output: &github.CheckRunOutput{
				Title:   &outputTitle,
				Summary: &outputSummary,
			},
		})
		if eror != nil {
			LogError.Errorf("github update check run failed: %v", eror)
		}
	}
}

// UpdateCheckRun updates the check run result with output message
func UpdateCheckRun(ctx context.Context, client *github.Client, gpull *GithubPull, checkRunID int64, conclusion string, t github.Timestamp, outputTitle string, outputSummary string, annotations []*github.CheckRunAnnotation) error {
	checkRunStatus := "completed"
	_, _, err := client.Checks.UpdateCheckRun(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name, checkRunID, github.UpdateCheckRunOptions{
		Name:        "linter",
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
func CreateCheckRun(ctx context.Context, client *github.Client, checkName string, gpull *GithubPull, ref GithubRef, targetURL string) (*github.CheckRun, error) {
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
