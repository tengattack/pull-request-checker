package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/util"
	githubhook "gopkg.in/rjz/githubhook.v0"
)

func webhookHandler(c *gin.Context) {
	hook, err := githubhook.Parse([]byte(common.Conf.API.WebHookSecret), c.Request)

	if err != nil {
		common.LogAccess.Errorf("Check signature error: " + err.Error())
		abortWithError(c, 403, "check signature error")
		return
	}

	common.LogAccess.Debugf("%s", hook.Payload)

	if hook.Event == "ping" {
		// pass
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"info": "Welcome to pull request checker server.",
		})
	} else if hook.Event == "pull_request" {
		var payload common.GithubWebHookPullRequest
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
		messagePrefix := fmt.Sprintf("%s/pull/%d/commits/", payload.Repository.FullName, payload.PullRequest.Number)
		message := messagePrefix + payload.PullRequest.Head.Sha
		common.LogAccess.WithField("entry", "webhook").Info("Push message: " + message)
		ref := common.GithubRef{
			Owner:    payload.Repository.Owner.Login,
			RepoName: payload.Repository.Name,

			Sha: payload.PullRequest.Head.Sha,
		}
		err = common.MQ.Push(message, messagePrefix, false)
		if err != nil {
			common.LogAccess.Error("Add message to queue error: " + err.Error())
			abortWithError(c, 500, "add to queue error: "+err.Error())
		} else {
			client, _, err := common.GetDefaultAPIClient(payload.Repository.Owner.Login)
			if err != nil {
				common.LogAccess.Errorf("getDefaultAPIClient returns error: %v", err)
				abortWithError(c, 500, "getDefaultAPIClient returns error")
				return
			}
			common.MarkAsPending(client, ref)
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"info": "add to queue successfully",
			})
		}
	} else if hook.Event == "check_run" {
		var payload common.GithubWebHookCheckRun
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

		client, _, err := common.GetDefaultAPIClient(payload.Repository.Owner.Login)
		if err != nil {
			common.LogAccess.Errorf("getDefaultAPIClient returns error: %v", err)
			abortWithError(c, 500, "getDefaultAPIClient returns error")
			return
		}
		prNum := 0
		if len(payload.CheckRun.PullRequests) > 0 {
			prNum = *payload.CheckRun.PullRequests[0].Number
		} else {
			prNum, err = common.SearchGithubPR(context.Background(), client, payload.Repository.FullName, *payload.CheckRun.HeadSHA)
			if err != nil {
				common.LogAccess.Errorf("SearchGithubPR error: %v", err)
				abortWithError(c, 404, "Could not get the PR number")
				return
			}
			if prNum == 0 {
				common.LogAccess.Infof("commit: %s no longer exists. No need to review.", *payload.CheckRun.HeadSHA)
				return
			}
		}

		messagePrefix := fmt.Sprintf("%s/pull/%d/commits/", payload.Repository.FullName, prNum)
		message := messagePrefix + *payload.CheckRun.HeadSHA
		common.LogAccess.WithField("entry", "webhook").Info("Push message: " + message)
		ref := common.GithubRef{
			Owner:    payload.Repository.Owner.Login,
			RepoName: payload.Repository.Name,

			Sha: *payload.CheckRun.HeadSHA,
		}
		err = common.MQ.Push(message, messagePrefix, false)
		if err != nil {
			common.LogAccess.Error("Add message to queue error: " + err.Error())
			abortWithError(c, 500, "add to queue error: "+err.Error())
		} else {
			common.MarkAsPending(client, ref)
			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"info": "add to queue successfully",
			})
		}
	} else {
		abortWithError(c, 415, "unsupported event: "+hook.Event)
	}
}

func addMessageHandler(c *gin.Context) {
	message := c.PostForm("message")
	if message == "" {
		urlParam := c.PostForm("url")
		if urlParam == "" {
			abortWithError(c, 400, "params error")
			return
		}
		u, err := url.Parse(urlParam)
		if err != nil {
			abortWithError(c, 400, "malformed url message")
			return
		}
		if u.Host != "github.com" {
			abortWithError(c, 400, fmt.Sprintf("unsupport url host: %s", u.Host))
			return
		}
		var branchName string
		var prNum int
		s := strings.Split(u.Path, "/")
		if s[0] != "" {
			// the message should starts with "/"
			abortWithError(c, 400, "malformed url message")
			return
		}
		s = s[1:] // strip the first "/"
		switch len(s) {
		case 2: // eg. https://github.com/tengattack/playground
			// check master
			branchName = "master"
		case 4: // eg. https://github.com/tengattack/playground/pull/2 or https://github.com/tengattack/playground/tree/master
			// check pull or branch
			if s[2] == "tree" {
				// branch
				branchName = s[3]
			} else {
				// pull
				prNum, err = strconv.Atoi(s[3])
				if err != nil {
					abortWithError(c, 400, "malformed url message")
					return
				}
			}
		case 6: // eg. https://github.com/tengattack/playground/pull/3/commits/73c5f8a45a4f02b595fbe1713ee3172749b7fc0c
			// default message path
			message = strings.Join(s, "/")
		default:
			abortWithError(c, 400, "malformed url message")
			return
		}

		if message == "" {
			ctx := context.Background()
			owner, repo := s[0], s[1]
			client, _, err := common.GetDefaultAPIClient(owner)
			if err != nil {
				common.LogAccess.Errorf("getDefaultAPIClient returns error: %v", err)
				abortWithError(c, 500, "getDefaultAPIClient returns error")
				return
			}
			if branchName != "" {
				// branch
				branch, _, err := client.Repositories.GetBranch(ctx, owner, repo, branchName)
				if err != nil {
					common.LogError.Errorf("checkProjects:GetBranch for %s error: %v", branchName, err)
					abortWithError(c, 500, fmt.Sprintf("GetBranch error: %v", err))
					return
				}
				message = fmt.Sprintf("%s/%s/tree/%s/commits/%s", owner, repo, branchName, *branch.Commit.SHA)
			} else {
				// pull
				pr, _, err := client.PullRequests.Get(ctx, owner, repo, prNum)
				if err != nil {
					common.LogError.Errorf("checkProjects:GetBranch for master error: %v", err)
					abortWithError(c, 500, fmt.Sprintf("GetPullRequests error: %v", err))
					return
				}
				if pr.GetState() != "open" {
					abortWithError(c, 404, "PR closed")
					return
				}
				message = fmt.Sprintf("%s/%s/pull/%d/commits/%s", owner, repo, prNum, *pr.Head.SHA)
			}
		}
	}

	m, err := util.ParseMessage(message)
	if err == util.ErrMalformedMessage {
		abortWithError(c, 400, "malformed message")
		return
	}
	if err != nil {
		abortWithError(c, 500, fmt.Sprintf("prase message error: %v", err))
		return
	}

	err = common.MQ.Push(message, m.Prefix(), true) // push to top
	if err != nil {
		abortWithError(c, 500, fmt.Sprintf("add message error: %v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"info": "add to queue successfully",
	})
}
