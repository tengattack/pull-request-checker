package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tengattack/unified-ci/common"
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
		err = common.MQ.Push(message, messagePrefix)
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
		err = common.MQ.Push(message, messagePrefix)
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
