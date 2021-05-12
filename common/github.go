package common

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/tengattack/unified-ci/mq"
)

const (
	GITHUB_API_URL = "https://api.github.com"
)

type githubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type GithubPull struct {
	URL    string     `json:"url"`
	ID     int64      `json:"id"`
	Number int64      `json:"number"`
	State  string     `json:"state"`
	Title  string     `json:"title"`
	Head   GithubRef  `json:"head"`
	Base   GithubRef  `json:"base"`
	User   githubUser `json:"user"`
}

type GithubRef struct {
	CheckType string
	CheckRef  string

	Owner    string
	RepoName string

	Repo struct {
		Name     string     `json:"name"`
		Owner    githubUser `json:"owner"`
		HTMLURL  string     `json:"html_url"`
		SSHURL   string     `json:"ssh_url"`
		CloneURL string     `json:"clone_url"`
	} `json:"repo"`
	Label   string     `json:"label"`
	Ref     string     `json:"ref"`
	BaseSha string     `json:"base_sha"`
	Sha     string     `json:"sha"`
	User    githubUser `json:"user"`
}

// IsBranch returns true if the checked type is checking of named branch such as master, stable.
func (ref GithubRef) IsBranch() bool {
	return ref.CheckType == CheckTypeBranch
}

type GithubRefReviewResponse struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	CommentID string `json:"commit_id"`
	State     string `json:"state"`
}

type GithubRepo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type GithubWebHookPullRequest struct {
	Action      string     `json:"action"`
	PullRequest GithubPull `json:"pull_request"`
	Repository  GithubRepo `json:"repository"`
}

// GithubWebHookCheckRun is the the request body of https://developer.github.com/v3/activity/events/types/#checkrunevent
type GithubWebHookCheckRun struct {
	Action     string          `json:"action"`
	CheckRun   github.CheckRun `json:"check_run"`
	Repository GithubRepo      `json:"repository"`
}

// GetGithubPulls gets the pull requests of the specified repository.
func GetGithubPulls(ctx context.Context, client *github.Client, owner, repo string) ([]*github.PullRequest, error) {
	opt := &github.PullRequestListOptions{}
	pulls, _, err := client.PullRequests.List(ctx, owner, repo, opt)
	if err != nil {
		LogError.Errorf("PullRequests.List returned error: %v", err)
		return nil, err
	}
	return pulls, nil
}

// GetGithubPull gets a single pull request.
func GetGithubPull(ctx context.Context, client *github.Client, owner, repo string, prNum int) (*github.PullRequest, error) {
	thePull, _, err := client.PullRequests.Get(ctx, owner, repo, prNum)
	if err != nil {
		LogError.Errorf("PullRequests.Get returned error: %v", err)
		return nil, err
	}
	return thePull, nil
}

// GetGithubPullDiff gets the diff of the pull request.
func GetGithubPullDiff(ctx context.Context, client *github.Client, owner, repo string, prNum int) ([]byte, error) {
	got, _, err := client.PullRequests.GetRaw(ctx, owner, repo, prNum, github.RawOptions{github.Diff})
	if err != nil {
		LogError.Errorf("PullRequests.GetRaw returned error: %v", err)
		return nil, err
	}
	return []byte(got), nil
}

// GetStatuses lists the statuses of a repository at the specified reference.
func (ref *GithubRef) GetStatuses(client *github.Client) ([]*github.RepoStatus, error) {
	statuses, _, err := client.Repositories.ListStatuses(context.Background(), ref.Owner, ref.RepoName, ref.Sha, nil)
	if err != nil {
		LogError.Errorf("Repositories.ListStatuses returned error: %v", err)
		return nil, err
	}
	return statuses, nil
}

// UpdateState creates the status
func (ref *GithubRef) UpdateState(client *github.Client, ctx, state, targetURL, description string) error {
	input := &github.RepoStatus{
		Context:     github.String(ctx),
		State:       github.String(state),
		TargetURL:   github.String(targetURL),
		Description: github.String(description),
	}
	_, _, err := client.Repositories.CreateStatus(context.Background(), ref.Owner, ref.RepoName, ref.Sha, input)
	if err != nil {
		LogError.Errorf("Repositories.CreateStatus returned error: %v", err)
		return err
	}
	return nil
}

// CreateReview creates a new review on the specified pull request.
func (ref *GithubRef) CreateReview(client *github.Client, prNum int, event, body string, comments []*github.DraftReviewComment) error {
	input := &github.PullRequestReviewRequest{
		CommitID: github.String(ref.Sha),
		Body:     github.String(body),
		Event:    github.String(event),
		Comments: comments,
	}

	_, _, err := client.PullRequests.CreateReview(context.Background(), ref.Owner, ref.RepoName, prNum, input)
	if err != nil {
		LogError.Errorf("PullRequests.CreateReview returned error: %v", err)
		return err
	}
	return nil
}

// GetDefaultAPIClient get default github api client
func GetDefaultAPIClient(owner string) (*github.Client, int64, error) {
	// Wrap the shared transport for use with the integration ID authenticating with installation ID.
	// TODO: add installation ID to db
	installationID, ok := Conf.GitHub.Installations[owner]
	if !ok {
		return nil, 0, fmt.Errorf("Installation ID not found, owner: %s", owner)
	}
	tr, err := newProxyRoundTripper()
	if err != nil {
		return nil, 0, err
	}
	tr, err = ghinstallation.NewKeyFromFile(tr,
		Conf.GitHub.AppID, installationID, Conf.GitHub.PrivateKey)
	if err != nil {
		return nil, 0, fmt.Errorf("Load private key failed: %v", err)
	}

	// TODO: refine code
	client := github.NewClient(&http.Client{Transport: tr})
	return client, installationID, nil
}

// HasLinterChecks check specified commit whether contain the linter checks
func HasLinterChecks(ref *GithubRef) (bool, error) {
	client, _, err := GetDefaultAPIClient(ref.Owner)
	if err != nil {
		LogError.Errorf("load private key failed: %v", err)
		return false, err
	}

	ctx := context.Background()
	checkRuns, _, err := client.Checks.ListCheckRunsForRef(ctx, ref.Owner, ref.RepoName, ref.Sha, nil)
	if err != nil {
		LogError.Errorf("github list check runs failed: %v", err)
		return false, err
	}

	for _, checkRun := range checkRuns.CheckRuns {
		if checkRun.GetName() == "linter" {
			return true, nil
		}
	}

	return false, nil
}

// HasLintStatuses check specified commit whether contain the lint context
func HasLintStatuses(client *github.Client, ref *GithubRef) (bool, error) {
	statuses, err := ref.GetStatuses(client)
	if err != nil {
		LogError.Errorf("github get statuses failed: %v", err)
		return false, err
	}
	lint := 0
	for _, s := range statuses {
		if s.GetContext() == AppName {
			lint++
		}
	}
	return lint > 0, nil
}

// NeedPRChecking check whether the PR is need checking
// TODO: add test
func NeedPRChecking(client *github.Client, ref *GithubRef, message string, MQ mq.MessageQueue) (bool, error) {
	statuses, err := ref.GetStatuses(client)
	if err != nil {
		err = fmt.Errorf("github get statuses failed: %v", err)
		return false, err
	}

	needCheck := true
	statusPending := false
	for _, v := range statuses {
		if v.GetContext() == AppName {
			switch v.GetState() {
			case "success", "error", "failure":
				needCheck = false
				return needCheck, nil
			case "pending":
				statusPending = true
			}
			break
		}
	}

	if statusPending {
		exist, err := MQ.Exists(message)
		if err != nil {
			return false, err
		}
		if exist {
			needCheck = false
		}
	}
	return needCheck, nil
}

// MarkAsPending mark github checks state as pending
func MarkAsPending(client *github.Client, ref GithubRef) {
	targetURL := ""
	if len(Conf.Core.CheckLogURI) > 0 {
		targetURL = Conf.Core.CheckLogURI + ref.Owner + "/" + ref.RepoName + "/" + ref.Sha + ".log"
	}
	err := ref.UpdateState(client, AppName, "pending", targetURL,
		"check queueing")
	if err != nil {
		LogAccess.Error("Update pull request status error: " + err.Error())
	}
}

// SizeLabel for pre-defined labels
type SizeLabel struct {
	Name        string
	Color       string
	Description string
	MaxLine     int
}

var sizeLabels = []*SizeLabel{
	&SizeLabel{Name: "size/XS", Color: "009900", Description: "Denotes a PR that changes 0-9 lines, ignoring generated files.", MaxLine: 9},
	&SizeLabel{Name: "size/S", Color: "77bb00", Description: "Denotes a PR that changes 10-29 lines, ignoring generated files.", MaxLine: 29},
	&SizeLabel{Name: "size/M", Color: "eebb00", Description: "Denotes a PR that changes 30-99 lines, ignoring generated files.", MaxLine: 99},
	&SizeLabel{Name: "size/L", Color: "ee9900", Description: "Denotes a PR that changes 100-499 lines, ignoring generated files.", MaxLine: 499},
	&SizeLabel{Name: "size/XL", Color: "ee5500", Description: "Denotes a PR that changes 500-999 lines, ignoring generated files.", MaxLine: 999},
	&SizeLabel{Name: "size/XXL", Color: "ee0000", Description: "Denotes a PR that changes 1000+ lines, ignoring generated files.", MaxLine: 0},
}

// LabelPRSize creates and labels PR with its size
func LabelPRSize(ctx context.Context, client *github.Client, ref GithubRef, prNum int, diffs []*diff.FileDiff) error {
	lines := 0
	for _, d := range diffs {
		for _, h := range d.Hunks {
			s := h.Stat()
			// REVIEW: changed needs to be doubled?
			lines += int(s.Added + s.Changed + s.Deleted)
		}
	}
	// LogAccess.Debugf("changed lines: %d", lines)

	sizeLabel := sizeLabels[0]
	for i := len(sizeLabels) - 1; i > 0; i-- {
		if lines > sizeLabels[i-1].MaxLine {
			sizeLabel = sizeLabels[i]
			break
		}
	}

	opts := &github.ListOptions{}
	var labelsToBeRemoved []string
	hasExpectedLabel := false
	// check whether exists
	for {
		ls, resp, err := client.Issues.ListLabelsByIssue(ctx, ref.Owner, ref.RepoName, prNum, opts)
		if err != nil {
			return err
		}
		for _, l := range ls {
			if strings.HasPrefix(*l.Name, "size/") {
				// already exists
				if sizeLabel.Name == *l.Name {
					hasExpectedLabel = true
				} else {
					labelsToBeRemoved = append(labelsToBeRemoved, *l.Name)
				}
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	for _, s := range labelsToBeRemoved {
		_, err := client.Issues.RemoveLabelForIssue(ctx, ref.Owner, ref.RepoName, prNum, s)
		if err != nil {
			LogError.Errorf("remove label %s error: %v", s, err)
			// PASS
		}
	}
	if hasExpectedLabel {
		return nil
	}

	labels, _, err := client.Issues.AddLabelsToIssue(ctx, ref.Owner, ref.RepoName, prNum, []string{sizeLabel.Name})
	for _, l := range labels {
		if *l.Name == sizeLabel.Name {
			if l.Color == nil || l.Description == nil || *l.Color != sizeLabel.Color || *l.Description != sizeLabel.Description {
				l.Color = &sizeLabel.Color
				l.Description = &sizeLabel.Description
				_, _, err2 := client.Issues.EditLabel(ctx, ref.Owner, ref.RepoName, *l.Name, l)
				if err2 != nil {
					LogError.Errorf("edit label error: %v", err2)
					// PASS
				}
			}
			break
		}
	}
	return err
}

// SearchGithubPR searches for the PR number of one commit
func SearchGithubPR(ctx context.Context, client *github.Client, repo, sha string) (int, error) {
	if sha == "" {
		return 0, errors.New("SHA is empty")
	}
	q := fmt.Sprintf("is:pr repo:%s SHA:%s", repo, sha)
	opts := &github.SearchOptions{Sort: "created", Order: "asc"}
	result, _, err := client.Search.Issues(ctx, q, opts)
	if err != nil {
		return 0, err
	}
	if len(result.Issues) == 0 {
		return 0, nil
	}
	return result.Issues[0].GetNumber(), nil
}
