package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	githubhook "gopkg.in/rjz/githubhook.v0"
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
	RepoName string `json:"-"`
	Repo     struct {
		Name     string     `json:"name"`
		Owner    githubUser `json:"owner"`
		HTMLURL  string     `json:"html_url"`
		SSHURL   string     `json:"ssh_url"`
		HTTPSURL string     `json:"clone_url"`
	} `json:"repo"`
	Label string     `json:"label"`
	Ref   string     `json:"ref"`
	Sha   string     `json:"sha"`
	User  githubUser `json:"user"`
}

type GithubRefState struct {
	Context     string `json:"context"`
	State       string `json:"state"`
	TargetURL   string `json:"target_url"`
	Description string `json:"description"`
}

type GithubRefComment struct {
	CommentID string `json:"commit_id,omitempty"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Position  int    `json:"position"`
}

type GithubRefReview struct {
	CommentID string             `json:"commit_id"`
	Body      string             `json:"body"`
	Event     string             `json:"event"`
	Comments  []GithubRefComment `json:"comments,omitempty"`
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
func GetGithubPulls(client *github.Client, owner, repo string) ([]*github.PullRequest, error) {
	opt := &github.PullRequestListOptions{}
	pulls, _, err := client.PullRequests.List(context.Background(), owner, repo, opt)
	if err != nil {
		LogError.Errorf("PullRequests.List returned error: %v", err)
		return nil, err
	}
	return pulls, nil
}

// GetGithubPull gets a single pull request.
func GetGithubPull(client *github.Client, owner, repo, pull string) (*github.PullRequest, error) {
	num, err := strconv.Atoi(pull)
	if err != nil {
		return nil, errors.New("bad PR number")
	}
	thePull, _, err := client.PullRequests.Get(context.Background(), owner, repo, num)
	if err != nil {
		LogError.Errorf("PullRequests.Get returned error: %v", err)
		return nil, err
	}
	return thePull, nil
}

// GetGithubPullDiff gets the diff of the pull request.
func GetGithubPullDiff(client *github.Client, owner, repo, pull string) ([]byte, error) {
	num, err := strconv.Atoi(pull)
	if err != nil {
		return nil, errors.New("bad PR number")
	}
	got, _, err := client.PullRequests.GetRaw(context.Background(), owner, repo, num, github.RawOptions{github.Diff})
	if err != nil {
		LogError.Errorf("PullRequests.GetRaw returned error: %v", err)
		return nil, err
	}
	return []byte(got), nil
}

// GetStatuses lists the statuses of a repository at the specified reference.
func (ref *GithubRef) GetStatuses(client *github.Client) ([]*github.RepoStatus, error) {
	parts := strings.Split(ref.RepoName, "/")
	if len(parts) < 2 {
		return nil, errors.New("bad repo name")
	}
	statuses, _, err := client.Repositories.ListStatuses(context.Background(), parts[0], parts[1], ref.Sha, nil)
	if err != nil {
		LogError.Errorf("Repositories.ListStatuses returned error: %v", err)
		return nil, err
	}
	return statuses, nil
}

// UpdateState creates the status
func (ref *GithubRef) UpdateState(client *github.Client, ctx, state, targetURL, description string) error {
	parts := strings.Split(ref.RepoName, "/")
	if len(parts) < 2 {
		return errors.New("bad repo name")
	}

	input := &github.RepoStatus{
		Context:     github.String(ctx),
		State:       github.String(state),
		TargetURL:   github.String(targetURL),
		Description: github.String(description),
	}
	_, _, err := client.Repositories.CreateStatus(context.Background(), parts[0], parts[1], ref.Sha, input)
	if err != nil {
		LogError.Errorf("Repositories.CreateStatus returned error: %v", err)
		return err
	}
	return nil
}

// CreateReview creates a new review on the specified pull request.
func (ref *GithubRef) CreateReview(client *github.Client, pull, event, body string, comments []*github.DraftReviewComment) error {
	num, err := strconv.Atoi(pull)
	if err != nil {
		return errors.New("bad PR number")
	}
	parts := strings.Split(ref.RepoName, "/")
	if len(parts) < 2 {
		return errors.New("bad repo name")
	}

	input := &github.PullRequestReviewRequest{
		CommitID: github.String(ref.Sha),
		Body:     github.String(body),
		Event:    github.String(event),
		Comments: comments,
	}

	_, _, err = client.PullRequests.CreateReview(context.Background(), parts[0], parts[1], num, input)
	if err != nil {
		LogError.Errorf("PullRequests.CreateReview returned error: %v", err)
		return err
	}
	return nil
}

func (ref *GithubRef) GetReviews(pull string) ([]GithubRefReviewResponse, error) {
	// GET /repos/:owner/:repo/pulls/:number/reviews
	apiURI := fmt.Sprintf("/repos/%s/pulls/%s/reviews", ref.RepoName, pull)

	query := url.Values{}
	query.Set("access_token", Conf.GitHub.AccessToken)

	LogAccess.Debugf("GET %s?%s", apiURI, query.Encode())

	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s%s?%s", GITHUB_API_URL, apiURI, query.Encode()), nil)
	if err != nil {
		return nil, err
	}

	var s []GithubRefReviewResponse
	err = DoHTTPRequest(req, true, &s)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (ref *GithubRef) SubmitReview(pull string, id int64, event, body string) error {
	data := struct {
		Event string `json:"event"`
		Body  string `json:"body"`
	}{
		Event: event,
		Body:  body,
	}

	// POST /repos/:owner/:repo/pulls/:number/reviews/:id/events
	apiURI := fmt.Sprintf("/repos/%s/pulls/%s/reviews/%d/events", ref.RepoName, pull, id)

	query := url.Values{}
	query.Set("access_token", Conf.GitHub.AccessToken)
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}

	LogAccess.Debugf("POST %s?%s\n%s", apiURI, query.Encode(), content)

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s%s?%s", GITHUB_API_URL, apiURI, query.Encode()),
		bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	var s GithubRefReviewResponse
	return DoHTTPRequest(req, true, &s)
}

func webhookHandler(c *gin.Context) {
	hook, err := githubhook.Parse([]byte(Conf.GitHub.Secret), c.Request)

	if err != nil {
		LogAccess.Errorf("Check signature error: " + err.Error())
		abortWithError(c, 403, "check signature error")
		return
	}

	LogAccess.Debugf("%s", hook.Payload)

	if hook.Event == "ping" {
		// pass
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"info": "Welcome to pull request checker server.",
		})
	} else if hook.Event == "pull_request" {
		var payload GithubWebHookPullRequest
		err = hook.Extract(&payload)
		if err != nil {
			abortWithError(c, 400, "payload error: "+err.Error())
			return
		}
		if payload.Action != "open" && payload.Action != "synchronize" {
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"info": "no need to handle the action: " + payload.Action,
			})
			return
		}
		// opend or synchronized
		message := fmt.Sprintf("%s/pull/%d/commits/%s",
			payload.Repository.FullName,
			payload.PullRequest.Number,
			payload.PullRequest.Head.Sha,
		)
		LogAccess.WithField("entry", "webhook").Info("Push message: " + message)
		ref := GithubRef{
			RepoName: payload.Repository.FullName,
			Sha:      payload.PullRequest.Head.Sha,
		}
		err = MQ.Push(message)
		if err != nil {
			LogAccess.Error("Add message to queue error: " + err.Error())
			abortWithError(c, 500, "add to queue error: "+err.Error())
		} else {
			client, err := getDefaultAPIClient(payload.Repository.Owner.Login, Conf.GitHub.AppID, Conf.GitHub.PrivateKey)
			if err != nil {
				LogAccess.Errorf("getDefaultAPIClient returns error: %v", err)
			}
			markAsPending(client, ref)
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"info": "add to queue successfully",
			})
		}
	} else if hook.Event == "check_run" {
		var payload GithubWebHookCheckRun
		err = hook.Extract(&payload)
		if err != nil {
			abortWithError(c, 400, "payload error: "+err.Error())
			return
		}
		if payload.Action != "rerequested" {
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"info": "no need to handle the action: " + payload.Action,
			})
			return
		}

		client, err := getDefaultAPIClient(payload.Repository.Owner.Login, Conf.GitHub.AppID, Conf.GitHub.PrivateKey)
		if err != nil {
			LogAccess.Errorf("getDefaultAPIClient returns error: %v", err)
			abortWithError(c, 500, "getDefaultAPIClient returns error")
			return
		}
		prNum := 0
		if len(payload.CheckRun.PullRequests) > 0 {
			prNum = *payload.CheckRun.PullRequests[0].Number
		} else {
			prNum, err = searchGithubPR(context.Background(), client, payload.Repository.FullName, *payload.CheckRun.HeadSHA)
			if err != nil {
				LogAccess.Errorf("searchGithubPR error: %v", err)
				abortWithError(c, 404, "Could not get the PR number")
				return
			}
		}

		message := fmt.Sprintf("%s/pull/%d/commits/%s",
			payload.Repository.FullName,
			prNum,
			*payload.CheckRun.HeadSHA,
		)
		LogAccess.WithField("entry", "webhook").Info("Push message: " + message)
		ref := GithubRef{
			RepoName: payload.Repository.FullName,
			Sha:      *payload.CheckRun.HeadSHA,
		}
		err = MQ.Push(message)
		if err != nil {
			LogAccess.Error("Add message to queue error: " + err.Error())
			abortWithError(c, 500, "add to queue error: "+err.Error())
		} else {
			markAsPending(client, ref)
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"info": "add to queue successfully",
			})
		}
	} else {
		abortWithError(c, 415, "unsupported event: "+hook.Event)
	}
}

// HasLinterChecks check specified commit whether contain the linter checks
func HasLinterChecks(ref *GithubRef) (bool, error) {
	parts := strings.Split(ref.RepoName, "/")
	client, err := getDefaultAPIClient(parts[0], Conf.GitHub.AppID, Conf.GitHub.PrivateKey)
	if err != nil {
		LogError.Errorf("load private key failed: %v", err)
		return false, err
	}

	ctx := context.Background()
	checkRuns, _, err := client.Checks.ListCheckRunsForRef(ctx, parts[0], parts[1], ref.Sha, nil)
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
		if s.GetContext() == "lint" {
			lint++
		}
	}
	return lint > 0, nil
}

func WatchLocalRepo() error {
	var err error
	for {
		files, err := ioutil.ReadDir(Conf.Core.WorkDir)
		if err != nil {
			break
		}
		for _, file := range files {
			if file.IsDir() {
				path := filepath.Join(Conf.Core.WorkDir, file.Name())
				subfiles, err := ioutil.ReadDir(path)
				if err != nil {
					break
				}
				client, err := getDefaultAPIClient(file.Name(), Conf.GitHub.AppID, Conf.GitHub.PrivateKey)
				if err != nil {
					break
				}
				for _, subfile := range subfiles {
					if subfile.IsDir() {
						repository := file.Name() + "/" + subfile.Name()
						pulls, err := GetGithubPulls(client, file.Name(), subfile.Name())
						if err != nil {
							continue
						}
						for _, pull := range pulls {
							ref := GithubRef{
								RepoName: repository,
								Sha:      pull.GetHead().GetSHA(),
							}
							exists, err := HasLintStatuses(client, &ref)
							if err != nil {
								continue
							}
							if !exists {
								exists, err = HasLinterChecks(&ref)
								if err != nil {
									continue
								}
							}
							if !exists {
								// no statuses, need check
								message := fmt.Sprintf("%s/pull/%d/commits/%s", ref.RepoName, pull.GetNumber(), ref.Sha)
								LogAccess.WithField("entry", "local").Info("Push message: " + message)
								err = MQ.Push(message)
								if err == nil {
									markAsPending(client, ref)
								} else {
									LogAccess.Error("Add message to queue error: " + err.Error())
								}
							}
						}
					}
				}
			}
		}
		time.Sleep(120 * time.Second)
	}
	if err != nil {
		LogAccess.Error("Local repo watcher error: " + err.Error())
	}
	return err
}

func markAsPending(client *github.Client, ref GithubRef) {
	targetURL := ""
	if len(Conf.Core.CheckLogURI) > 0 {
		targetURL = Conf.Core.CheckLogURI + ref.RepoName + "/" + ref.Sha + ".log"
	}
	err := ref.UpdateState(client, "lint", "pending", targetURL,
		"check queueing")
	if err != nil {
		LogAccess.Error("Update pull request status error: " + err.Error())
	}
}
