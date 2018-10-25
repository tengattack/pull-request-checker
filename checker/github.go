package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
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
		HTMLURL string `json:"html_url"`
		SSHURL  string `json:"ssh_url"`
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
}

type GithubWebHookPullRequest struct {
	Action      string     `json:"action"`
	PullRequest GithubPull `json:"pull_request"`
	Repository  GithubRepo `json:"repository"`
}

func GetGithubPulls(repo string) ([]GithubPull, error) {
	apiURI := fmt.Sprintf("/repos/%s/pulls", repo)

	query := url.Values{}
	query.Set("access_token", Conf.GitHub.AccessToken)

	LogAccess.Debugf("GET %s?%s", apiURI, query.Encode())

	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s%s?%s", GITHUB_API_URL, apiURI, query.Encode()), nil)
	if err != nil {
		return nil, err
	}

	var resp []GithubPull
	err = DoHTTPRequest(req, true, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func GetGithubPull(repo, pull string) (*GithubPull, error) {
	apiURI := fmt.Sprintf("/repos/%s/pulls/%s", repo, pull)

	query := url.Values{}
	query.Set("access_token", Conf.GitHub.AccessToken)

	LogAccess.Debugf("GET %s?%s", apiURI, query.Encode())

	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s%s?%s", GITHUB_API_URL, apiURI, query.Encode()), nil)
	if err != nil {
		return nil, err
	}

	var resp GithubPull
	err = DoHTTPRequest(req, true, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func GetGithubPullDiff(repo, pull string) ([]byte, error) {
	apiURI := fmt.Sprintf("/repos/%s/pulls/%s", repo, pull)

	query := url.Values{}
	query.Set("access_token", Conf.GitHub.AccessToken)

	LogAccess.Debugf("GET %s?%s", apiURI, query.Encode())

	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s%s?%s", GITHUB_API_URL, apiURI, query.Encode()), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	var resp []byte
	err = DoHTTPRequest(req, false, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (ref *GithubRef) GetStatuses() ([]GithubRefState, error) {
	apiURI := fmt.Sprintf("/repos/%s/commits/%s/statuses", ref.RepoName, ref.Sha)
	query := url.Values{}
	query.Set("access_token", Conf.GitHub.AccessToken)

	LogAccess.Debugf("GET %s?%s", apiURI, query.Encode())

	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s%s?%s", GITHUB_API_URL, apiURI, query.Encode()), nil)
	if err != nil {
		return nil, err
	}

	var s []GithubRefState
	err = DoHTTPRequest(req, true, &s)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (ref *GithubRef) UpdateState(context, state, targetURL, description string) error {
	data := GithubRefState{
		Context:     context,
		State:       state,
		TargetURL:   targetURL,
		Description: description,
	}

	apiURI := fmt.Sprintf("/repos/%s/statuses/%s", ref.RepoName, ref.Sha)

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

	var s GithubRef
	return DoHTTPRequest(req, true, &s)
}

func (ref *GithubRef) CreateComment(pull, path string, position int, body string) error {
	data := GithubRefComment{
		CommentID: ref.Sha,
		Body:      body,
		Path:      path,
		Position:  position,
	}

	// /repos/:owner/:repo/pulls/:number/comments
	apiURI := fmt.Sprintf("/repos/%s/pulls/%s/comments", ref.RepoName, pull)

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

	var s GithubRefComment
	return DoHTTPRequest(req, true, &s)
}

func (ref *GithubRef) CreateReview(pull, event, body string, comments []GithubRefComment) error {
	data := GithubRefReview{
		CommentID: ref.Sha,
		Body:      body,
		Event:     event,
		Comments:  comments,
	}

	// POST /repos/:owner/:repo/pulls/:number/reviews
	apiURI := fmt.Sprintf("/repos/%s/pulls/%s/reviews", ref.RepoName, pull)

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

	var s GithubRefReview
	return DoHTTPRequest(req, true, &s)
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
			markAsPending(ref)
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"info": "add to queue successfully",
			})
		}
	} else {
		abortWithError(c, 415, "unsupported event: "+hook.Event)
	}
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
				for _, subfile := range subfiles {
					if subfile.IsDir() {
						repository := file.Name() + "/" + subfile.Name()
						pulls, err := GetGithubPulls(repository)
						if err != nil {
							continue
						}
						for _, pull := range pulls {
							ref := GithubRef{
								RepoName: repository,
								Sha:      pull.Head.Sha,
							}
							statuses, err := ref.GetStatuses()
							if err != nil {
								continue
							}
							lint := 0
							for _, s := range statuses {
								if s.Context == "lint" {
									lint++
								}
							}
							if lint <= 0 {
								// no statuses, need check
								message := fmt.Sprintf("%s/pull/%d/commits/%s", ref.RepoName, pull.Number, ref.Sha)
								LogAccess.WithField("entry", "localRepo").Info("Push message: " + message)
								err = MQ.Push(message)
								if err == nil {
									markAsPending(ref)
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

func markAsPending(ref GithubRef) {
	targetURL := ""
	if len(Conf.Core.CheckLogURI) > 0 {
		targetURL = Conf.Core.CheckLogURI + ref.RepoName + "/" + ref.Sha + ".log"
	}
	err := ref.UpdateState("lint", "pending", targetURL,
		"check queueing")
	if err != nil {
		LogAccess.Error("Update pull request status error: " + err.Error())
	}
}
