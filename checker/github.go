package checker

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/github"
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
	owner string
	repo  string

	Repo struct {
		Name     string     `json:"name"`
		Owner    githubUser `json:"owner"`
		HTMLURL  string     `json:"html_url"`
		SSHURL   string     `json:"ssh_url"`
		CloneURL string     `json:"clone_url"`
	} `json:"repo"`
	Label string     `json:"label"`
	Ref   string     `json:"ref"`
	Sha   string     `json:"sha"`
	User  githubUser `json:"user"`
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
func GetGithubPull(client *github.Client, owner, repo string, prNum int) (*github.PullRequest, error) {
	thePull, _, err := client.PullRequests.Get(context.Background(), owner, repo, prNum)
	if err != nil {
		LogError.Errorf("PullRequests.Get returned error: %v", err)
		return nil, err
	}
	return thePull, nil
}

// GetGithubPullDiff gets the diff of the pull request.
func GetGithubPullDiff(client *github.Client, owner, repo string, prNum int) ([]byte, error) {
	got, _, err := client.PullRequests.GetRaw(context.Background(), owner, repo, prNum, github.RawOptions{github.Diff})
	if err != nil {
		LogError.Errorf("PullRequests.GetRaw returned error: %v", err)
		return nil, err
	}
	return []byte(got), nil
}

// GetStatuses lists the statuses of a repository at the specified reference.
func (ref *GithubRef) GetStatuses(client *github.Client) ([]*github.RepoStatus, error) {
	statuses, _, err := client.Repositories.ListStatuses(context.Background(), ref.owner, ref.repo, ref.Sha, nil)
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
	_, _, err := client.Repositories.CreateStatus(context.Background(), ref.owner, ref.repo, ref.Sha, input)
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

	_, _, err := client.PullRequests.CreateReview(context.Background(), ref.owner, ref.repo, prNum, input)
	if err != nil {
		LogError.Errorf("PullRequests.CreateReview returned error: %v", err)
		return err
	}
	return nil
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
			owner: payload.Repository.Owner.Login,
			repo:  payload.Repository.Name,

			Sha: payload.PullRequest.Head.Sha,
		}
		err = MQ.Push(message)
		if err != nil {
			LogAccess.Error("Add message to queue error: " + err.Error())
			abortWithError(c, 500, "add to queue error: "+err.Error())
		} else {
			client, err := getDefaultAPIClient(payload.Repository.Owner.Login)
			if err != nil {
				LogAccess.Errorf("getDefaultAPIClient returns error: %v", err)
				abortWithError(c, 500, "getDefaultAPIClient returns error")
				return
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

		client, err := getDefaultAPIClient(payload.Repository.Owner.Login)
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
			if prNum == 0 {
				LogAccess.Infof("%s no longer exists. No need to review.", *payload.CheckRun.HeadSHA)
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
			owner: payload.Repository.Owner.Login,
			repo:  payload.Repository.Name,

			Sha: *payload.CheckRun.HeadSHA,
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
	client, err := getDefaultAPIClient(ref.owner)
	if err != nil {
		LogError.Errorf("load private key failed: %v", err)
		return false, err
	}

	ctx := context.Background()
	checkRuns, _, err := client.Checks.ListCheckRunsForRef(ctx, ref.owner, ref.repo, ref.Sha, nil)
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

func WatchLocalRepo() error {
	var err error
	for {
		files, err := ioutil.ReadDir(Conf.Core.WorkDir)
		if err != nil {
			return err
		}
		for _, file := range files {
			isDir := file.IsDir()
			path := filepath.Join(Conf.Core.WorkDir, file.Name())
			if !isDir && file.Mode()&os.ModeSymlink == os.ModeSymlink {
				realPath, err := os.Readlink(path)
				if err != nil {
					continue
				}
				st, err := os.Stat(realPath)
				if err != nil {
					continue
				}
				if st.IsDir() {
					isDir = true
					path = realPath
				}
			}
			if isDir {
				subfiles, err := ioutil.ReadDir(path)
				if err != nil {
					continue
				}
				client, err := getDefaultAPIClient(file.Name())
				if err != nil {
					continue
				}
				for _, subfile := range subfiles {
					isDir := subfile.IsDir()
					if !isDir && subfile.Mode()&os.ModeSymlink == os.ModeSymlink {
						realPath, err := os.Readlink(filepath.Join(path, subfile.Name()))
						if err != nil {
							continue
						}
						st, err := os.Stat(realPath)
						if err != nil {
							continue
						}
						if st.IsDir() {
							isDir = true
						}
					}
					if isDir {
						pulls, err := GetGithubPulls(client, file.Name(), subfile.Name())
						if err != nil {
							LogError.Errorf("WatchLocalRepo:GetGithubPulls: %v", err)
							continue
						}
						for _, pull := range pulls {
							ref := GithubRef{
								owner: file.Name(),
								repo:  subfile.Name(),

								Sha: pull.GetHead().GetSHA(),
							}
							exists, err := HasLintStatuses(client, &ref)
							if err != nil {
								LogError.Errorf("WatchLocalRepo:HasLintStatuses: %v", err)
								continue
							}
							if !exists {
								exists, err = HasLinterChecks(&ref)
								if err != nil {
									LogError.Errorf("WatchLocalRepo:HasLinterChecks: %v", err)
									continue
								}
							}
							if !exists {
								// no statuses, need check
								message := fmt.Sprintf("%s/%s/pull/%d/commits/%s", ref.owner, ref.repo, pull.GetNumber(), ref.Sha)
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
		targetURL = Conf.Core.CheckLogURI + ref.owner + "/" + ref.repo + "/" + ref.Sha + ".log"
	}
	err := ref.UpdateState(client, AppName, "pending", targetURL,
		"check queueing")
	if err != nil {
		LogAccess.Error("Update pull request status error: " + err.Error())
	}
}
