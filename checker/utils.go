package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"time"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/util"
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
	req.Header.Set("User-Agent", common.UserAgent())
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

	common.LogAccess.Debugf("HTTP %s\n%s", resp.Status, body)

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
			common.LogError.Errorf("github update check run with error failed: %v", erro)
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
		common.LogError.Warn("The output summary is too long.")
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
		common.LogError.Errorf("github update check run failed: %v", err)
	}
	return err
}

// CreateCheckRun creates a new check run
func CreateCheckRun(ctx context.Context, client *github.Client, gpull *github.PullRequest, checkName string, ref common.GithubRef, targetURL string) (*github.CheckRun, error) {
	checkRunStatus := "in_progress"

	t := github.Timestamp{Time: time.Now()}
	owner := gpull.GetBase().GetRepo().GetOwner().GetLogin()
	repo := gpull.GetBase().GetRepo().GetName()
	checkRun, _, err := client.Checks.CreateCheckRun(ctx, owner, repo, github.CreateCheckRunOptions{
		Name:       checkName,
		HeadSHA:    ref.Sha,
		DetailsURL: &targetURL,
		Status:     &checkRunStatus,
		StartedAt:  &t,
	})
	return checkRun, err
}

// FibonacciBinet calculates fibonacci value by analytic (Binet's formula)
func FibonacciBinet(num int64) int64 {
	n := float64(num)
	return int64(((math.Pow(((1+math.Sqrt(5))/2), n) - math.Pow(1-((1+math.Sqrt(5))/2), n)) / math.Sqrt(5)) + 0.5)
}
