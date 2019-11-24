package checker

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/github"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/tengattack/unified-ci/mq"
	"github.com/tengattack/unified-ci/store"
	"github.com/tengattack/unified-ci/util"
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
	checkType string
	owner     string
	repo      string

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
			build = "failed"
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
	buildTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="88" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h51v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="615" y="140" transform="scale(.1)" textLength="410">%s</text></g> </svg>`
	buildFailedTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="76" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h39v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="555" y="140" transform="scale(.1)" textLength="290">%s</text></g> </svg>`
	buildUnknownTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="98" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h37v20H0z"/><path fill="%s" d="M37 0h61v20H37z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="185" y="140" transform="scale(.1)" textLength="270">build</text><text x="665" y="140" transform="scale(.1)" textLength="510">%s</text></g> </svg>`
	coverageTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="98" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h61v20H0z"/><path fill="%s" d="M61 0h37v20H61z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="305" y="140" transform="scale(.1)" textLength="510">coverage</text><text x="785" y="140" transform="scale(.1)" textLength="270">%s</text></g> </svg>`
	coverageUnknownTemplate := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="122" height="20"><g shape-rendering="crispEdges"><path fill="#555" d="M0 0h61v20H0z"/><path fill="%s" d="M61 0h61v20H61z"/></g><g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110"> <text x="305" y="140" transform="scale(.1)" textLength="510">coverage</text><text x="905" y="140" transform="scale(.1)" textLength="510">%s</text></g> </svg>`

	switch badgeType {
	case "build.svg":
		switch build {
		case "passing":
			color = colors[0]
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(buildTemplate, color, build)))
		case "failed":
			color = colors[len(colors)-1]
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(buildFailedTemplate, color, build)))
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
		} else {
			color = colors[5]
		}
		c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(fmt.Sprintf(coverageTemplate, color, coverage)))
	default:
		panic("unexpected params")
	}
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
			prNum, err = util.SearchGithubPR(context.Background(), client, payload.Repository.FullName, *payload.CheckRun.HeadSHA)
			if err != nil {
				LogAccess.Errorf("SearchGithubPR error: %v", err)
				abortWithError(c, 404, "Could not get the PR number")
				return
			}
			if prNum == 0 {
				LogAccess.Infof("commit: %s no longer exists. No need to review.", *payload.CheckRun.HeadSHA)
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

// TODO: add test
func needPRChecking(client *github.Client, ref *GithubRef, message string, MQ mq.MessageQueue) (bool, error) {
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

// WatchLocalRepo scans local repo periodically and sends a checking request if a opened PR hasn't any checks
func WatchLocalRepo(ctx context.Context) error {
	var err error
	for {
		files, err := ioutil.ReadDir(Conf.Core.WorkDir)
		if err != nil {
			return err
		}
		for _, file := range files {
			select {
			case <-ctx.Done():
				LogAccess.Warn("WatchLocalRepo canceled.")
				return nil
			default:
			}
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
					select {
					case <-ctx.Done():
						LogAccess.Warn("WatchLocalRepo canceled.")
						return nil
					default:
					}
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
						owner, repo := file.Name(), subfile.Name()
						projConf, err := readProjectConfig(filepath.Join(path, subfile.Name()))
						if err == nil && len(projConf.Tests) > 0 {
							masterBranch, _, err := client.Repositories.GetBranch(ctx, owner, repo, "master")
							if err != nil {
								LogError.Errorf("WatchLocalRepo:GetBranch for master error: %v", err)
								// PASS
							} else {
								// check master commit status
								masterCommitSHA := *masterBranch.Commit.SHA
								commitInfos, err := store.ListCommitsInfo(owner, repo, masterCommitSHA)
								if err != nil {
									LogError.Errorf("WatchLocalRepo:LoadCommitsInfo for master error: %v", err)
									// PASS
								} else if len(commitInfos) >= len(projConf.Tests) {
									// promote status
									updated := false
									for _, commitInfo := range commitInfos {
										if commitInfo.Status == 0 {
											err = commitInfo.UpdateStatus(1)
											if err != nil {
												LogError.Errorf("WatchLocalRepo:CommitInfo:UpdateStatus error: %v", err)
												// PASS
											} else {
												updated = true
											}
										}
									}
									if updated {
										LogAccess.Infof("CommitInfo %s/%s %s for master status updated", owner, repo, masterCommitSHA)
									}
								} else {
									ref := GithubRef{
										owner: owner,
										repo:  repo,

										Sha: masterCommitSHA,
									}
									message := fmt.Sprintf("%s/%s/tree/%s/commits/%s", ref.owner, ref.repo, "master", masterCommitSHA)
									needCheck, err := needPRChecking(client, &ref, message, MQ)
									if err != nil {
										LogError.Errorf("WatchLocalRepo:NeedPRChecking for master error: %v", err)
										continue
									}
									if needCheck {
										// no statuses, need check
										LogAccess.WithField("entry", "local").Info("Push message: " + message)
										err = MQ.Push(message)
										if err == nil {
											markAsPending(client, ref)
										} else {
											LogAccess.Error("Add message to queue error: " + err.Error())
											// PASS
										}
									}
								}
							}
						}
						pulls, err := GetGithubPulls(ctx, client, owner, repo)
						if err != nil {
							LogError.Errorf("WatchLocalRepo:GetGithubPulls error: %v", err)
							continue
						}
						for _, pull := range pulls {
							select {
							case <-ctx.Done():
								LogAccess.Warn("WatchLocalRepo canceled.")
								return nil
							default:
							}
							ref := GithubRef{
								owner: owner,
								repo:  repo,

								Sha: pull.GetHead().GetSHA(),
							}
							message := fmt.Sprintf("%s/%s/pull/%d/commits/%s", ref.owner, ref.repo, pull.GetNumber(), ref.Sha)
							needCheck, err := needPRChecking(client, &ref, message, MQ)
							if err != nil {
								LogError.Errorf("WatchLocalRepo:NeedPRChecking error: %v", err)
								continue
							}
							if needCheck {
								// no statuses, need check
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
		select {
		case <-ctx.Done():
			LogAccess.Warn("WatchLocalRepo canceled.")
			return nil
		case <-time.After(120 * time.Second):
		}
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
		ls, resp, err := client.Issues.ListLabelsByIssue(ctx, ref.owner, ref.repo, prNum, opts)
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
		_, err := client.Issues.RemoveLabelForIssue(ctx, ref.owner, ref.repo, prNum, s)
		if err != nil {
			LogError.Errorf("remove label %s error: %v", s, err)
			// PASS
		}
	}
	if hasExpectedLabel {
		return nil
	}

	labels, _, err := client.Issues.AddLabelsToIssue(ctx, ref.owner, ref.repo, prNum, []string{sizeLabel.Name})
	for _, l := range labels {
		if *l.Name == sizeLabel.Name {
			if l.Color == nil || l.Description == nil || *l.Color != sizeLabel.Color || *l.Description != sizeLabel.Description {
				l.Color = &sizeLabel.Color
				l.Description = &sizeLabel.Description
				_, _, err2 := client.Issues.EditLabel(ctx, ref.owner, ref.repo, *l.Name, l)
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
