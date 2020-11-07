package checker

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/store"
	githubhook "gopkg.in/rjz/githubhook.v0"
)

func badgesHandler(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")
	badgeType := c.Param("type")

	switch badgeType {
	case "build.svg":
	case "coverage.svg":
	default:
		abortWithError(c, 400, "error params")
		return
	}

	commitsInfo, err := store.GetLatestCommitsInfo(owner, repo)
	if err != nil {
		abortWithError(c, 500, "get latest commits info for "+owner+"/"+repo+" error: "+err.Error())
		return
	}

	build := "unknown"
	coverage := "unknown"
	for _, info := range commitsInfo {
		if info.Passing == 1 {
			build = "passing"
		} else {
			build = "failing"
			break
		}
	}

	var coverageNum int
	if len(commitsInfo) > 0 {
		var pct float64
		coverage = ""
		for _, info := range commitsInfo {
			if info.Coverage == nil {
				coverage = "unknown"
				break
			} else {
				pct += *info.Coverage
			}
		}
		if coverage == "" {
			coverageNum = int(math.Round(100 * pct / float64(len(commitsInfo))))
			coverage = strconv.Itoa(coverageNum) + "%"
		}
	}

	var color string
	unknownColor := "#9f9f9f"
	colors := []string{"#4c1", "#97ca00", "#a4a61d", "#dfb317", "#fe7d37", "#e05d44"}
	// TODO: common template
	buildTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="88" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h51v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="615" y="140" transform="scale(.1)" textLength="410">%s</text></g> </svg>`
	buildFailingTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="80" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h43v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="575" y="140" transform="scale(.1)" textLength="330">%s</text></g> </svg>`
	buildUnknownTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="98" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h61v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="665" y="140" transform="scale(.1)" textLength="510">%s</text></g> </svg>`
	coverageTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="98" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h61v20H0z"/><path fill="%s" d="M61 0h37v20H61z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="305" y="140" transform="scale(.1)" textLength="510">coverage</text><text x="785" y="140" transform="scale(.1)" textLength="270">%s</text></g> </svg>`
	coverageSmallTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="90" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h61v20H0z"/><path fill="%s" d="M61 0h29v20H61z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="305" y="140" transform="scale(.1)" textLength="510">coverage</text><text x="745" y="140" transform="scale(.1)" textLength="190">%s</text></g> </svg>`
	coverageUnknownTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="122" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h61v20H0z"/><path fill="%s" d="M61 0h61v20H61z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="305" y="140" transform="scale(.1)" textLength="510">coverage</text><text x="905" y="140" transform="scale(.1)" textLength="510">%s</text></g> </svg>`

	// make camo do not cache our responses
	c.Header("Cache-Control", "no-cache, max-age=0")

	switch badgeType {
	case "build.svg":
		switch build {
		case "passing":
			color = colors[0]
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(buildTemplate, color, build)))
		case "failing":
			color = colors[len(colors)-1]
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(buildFailingTemplate, color, build)))
		default:
			// unknown
			color = unknownColor
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(buildUnknownTemplate, color, build)))
		}
	case "coverage.svg":
		if coverage == "unknown" {
			color = unknownColor
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(coverageUnknownTemplate, color, coverage)))
			return
		}

		if coverageNum >= 93 { // or starts from 97%
			color = colors[0]
		} else if coverageNum >= 80 {
			color = colors[1]
		} else if coverageNum >= 65 {
			color = colors[2]
		} else if coverageNum >= 45 {
			color = colors[3]
		} else if coverageNum >= 15 {
			color = colors[4]
		} else if coverageNum >= 10 {
			color = colors[5]
		} else {
			// small coverage
			color = colors[5]
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(coverageSmallTemplate, color, coverage)))
			return
		}
		c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(coverageTemplate, color, coverage)))
	default:
		panic("unexpected params")
	}
}

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

func storePromoteStatus(ref common.GithubRef) (bool, error) {
	// check master commit status
	commitInfos, err := store.ListCommitsInfo(ref.Owner, ref.RepoName, ref.Sha)
	if err != nil {
		common.LogError.Errorf("WatchLocalRepo:LoadCommitsInfo for master error: %v", err)
		return false, err
	}
	// promote status
	updated := false
	for _, commitInfo := range commitInfos {
		if commitInfo.Status == 0 {
			err = commitInfo.UpdateStatus(1)
			if err != nil {
				common.LogError.Errorf("WatchLocalRepo:CommitInfo:UpdateStatus error: %v", err)
				// PASS
			} else {
				updated = true
			}
		}
	}
	if updated {
		common.LogAccess.Infof("CommitInfo %s/%s %s for master status updated", ref.Owner, ref.RepoName, ref.Sha)
	}
	return updated, nil
}

func checkProjects(ctx context.Context, projects []WorkerProjectConfig, enablePromote bool) {
	for _, project := range projects {
		parts := strings.Split(project.Name, "/")
		if len(parts) != 2 {
			continue
		}
		owner, repo := parts[0], parts[1]
		client, _, err := common.GetDefaultAPIClient(owner)
		if project.CheckMaster {
			masterBranch, _, err := client.Repositories.GetBranch(ctx, owner, repo, "master")
			if err != nil {
				common.LogError.Errorf("checkProjects:GetBranch for master error: %v", err)
				// PASS
			} else {
				// check master commit status
				masterCommitSHA := *masterBranch.Commit.SHA
				ref := common.GithubRef{
					Owner:    owner,
					RepoName: repo,

					Sha: masterCommitSHA,
				}
				updated := false
				if enablePromote {
					updated, _ = storePromoteStatus(ref)
				}
				if !updated {
					messagePrefix := fmt.Sprintf("%s/%s/tree/%s/commits/", ref.Owner, ref.RepoName, "master")
					message := messagePrefix + masterCommitSHA
					needCheck, err := common.NeedPRChecking(client, &ref, message, common.MQ)
					if err != nil {
						common.LogError.Errorf("checkProjects:NeedPRChecking for master error: %v", err)
						continue
					}
					if needCheck {
						// no statuses, need check
						common.LogAccess.WithField("entry", "local").Info("Push message: " + message)
						err = common.MQ.Push(message, messagePrefix)
						if err == nil {
							common.MarkAsPending(client, ref)
						} else {
							common.LogAccess.Error("Add message to queue error: " + err.Error())
							// PASS
						}
					}
				}
			}
		}
		pulls, err := common.GetGithubPulls(ctx, client, owner, repo)
		if err != nil {
			common.LogError.Errorf("checkProjects:GetGithubPulls error: %v", err)
			continue
		}
		for _, pull := range pulls {
			select {
			case <-ctx.Done():
				common.LogAccess.Warn("checkProjects canceled.")
				return
			default:
			}
			ref := common.GithubRef{
				Owner:    owner,
				RepoName: repo,

				Sha: pull.GetHead().GetSHA(),
			}
			messagePrefix := fmt.Sprintf("%s/%s/pull/%d/commits/", ref.Owner, ref.RepoName, pull.GetNumber())
			message := messagePrefix + ref.Sha
			needCheck, err := common.NeedPRChecking(client, &ref, message, common.MQ)
			if err != nil {
				common.LogError.Errorf("checkProjects:NeedPRChecking error: %v", err)
				continue
			}
			if needCheck {
				// no statuses, need check
				common.LogAccess.WithField("entry", "local").Info("Push message: " + message)
				err = common.MQ.Push(message, messagePrefix)
				if err == nil {
					common.MarkAsPending(client, ref)
				} else {
					common.LogAccess.Error("Add message to queue error: " + err.Error())
				}
			}
		}
	}
}

// WatchLocalRepo scans local repo periodically and sends a checking request if a opened PR hasn't any checks
func WatchLocalRepo(ctx context.Context) error {
	var err error
	for {
		select {
		case <-ctx.Done():
			common.LogAccess.Warn("WatchLocalRepo canceled.")
			return nil
		case <-time.After(60 * time.Second):
		}
		var projects []WorkerProjectConfig
		sw.Range(func(key, value interface{}) bool {
			projects = append(projects, value.(ServerWorker).Projects...)
			return true
		})
		if len(projects) <= 0 {
			continue
		}
		checkProjects(ctx, projects, true)
	}
	if err != nil {
		common.LogAccess.Error("Local repo watcher error: " + err.Error())
	}
	return err
}

// WatchServerWorkerRepo scans server workers' repo periodically and sends a checking request if a opened PR hasn't any checks
func WatchServerWorkerRepo(ctx context.Context) error {
	var err error
	for {
		select {
		case <-ctx.Done():
			common.LogAccess.Warn("WatchLocalRepo canceled.")
			return nil
		case <-time.After(60 * time.Second):
		}
		projects, err := getLocalRepo()
		if err != nil {
			common.LogError.Errorf("WatchLocalRepo:getLocalRepo error: %v", err)
			continue
			// PASS
		}
		checkProjects(ctx, projects, false)
	}
	if err != nil {
		common.LogAccess.Error("Local repo watcher error: " + err.Error())
	}
	return err
}
