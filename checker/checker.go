package checker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/go-github/github"
	"github.com/tengattack/unified-ci/checks/vulnerability"
	"github.com/tengattack/unified-ci/common"
)

// VulnerabilityCheckRun checks and reports package vulnerability.
func VulnerabilityCheckRun(ctx context.Context, client *github.Client, gpull *github.PullRequest, ref common.GithubRef,
	repoPath string, targetURL string, log io.Writer) (int, error) {
	const checkName = "vulnerability"
	var checkRunID int64

	if ref.IsBranch() {
		err := ref.UpdateState(client, checkName, "pending", targetURL, "running")
		if err != nil {
			msg := fmt.Sprintf("Update commit state %s failed: %v", checkName, err)
			_, _ = io.WriteString(log, msg+"\n")
			common.LogError.Error(msg)
			// PASS
		}
	} else {
		checkRun, err := CreateCheckRun(ctx, client, gpull, checkName, ref, targetURL)
		if err != nil {
			msg := fmt.Sprintf("Creating %s check run failed: %v", checkName, err)
			_, _ = io.WriteString(log, msg+"\n")
			common.LogError.Error(msg)
			// PASS
		} else {
			checkRunID = checkRun.GetID()
		}
	}

	data, err := vulnerability.CheckVulnerability(ref.RepoName, repoPath, ref.Sha, ref.CheckRef)
	if err != nil {
		msg := fmt.Sprintf("checks package vulnerability failed: %v", err)
		_, _ = io.WriteString(log, msg+"\n")
		common.LogError.Error(msg)
		if ref.IsBranch() {
			err := ref.UpdateState(client, checkName, "failure", targetURL, "")
			if err != nil {
				msg := fmt.Sprintf("Update commit state %s failed: %v", checkName, err)
				_, _ = io.WriteString(log, msg+"\n")
				common.LogError.Error(msg)
				// PASS
			}
		} else {
			if checkRunID != 0 {
				UpdateCheckRunWithError(ctx, client, gpull, checkRunID, checkName, checkName, err)
			}
		}
		return 0, err
	}

	if ref.IsBranch() {
		state := "success"
		title := "no vulnerabilities"
		if len(data) > 0 {
			state = "error"
			title = fmt.Sprintf("%d problem(s) found.", len(data))
		}
		err := ref.UpdateState(client, checkName, state, targetURL, title)
		if err != nil {
			msg := fmt.Sprintf("Update commit state %s failed: %v", checkName, err)
			_, _ = io.WriteString(log, msg+"\n")
			common.LogError.Error(msg)
			// PASS
		}
	} else {
		if checkRunID == 0 {
			checkRun, err := CreateCheckRun(ctx, client, gpull, checkName, ref, targetURL)
			if err != nil {
				msg := fmt.Sprintf("Creating %s check run failed: %v", checkName, err)
				_, _ = io.WriteString(log, msg+"\n")
				common.LogError.Error(msg)
				return 0, err
			}
			checkRunID = checkRun.GetID()
		}

		conclusion := "success"
		message := "no vulnerabilities"
		if len(data) > 0 {
			conclusion = "failure"
			message = data[0].MDTitle()
			for _, v := range data {
				message += v.MDTableRow()
			}
		}

		t := github.Timestamp{Time: time.Now()}
		err = UpdateCheckRun(ctx, client, gpull, checkRunID, checkName, conclusion, t, conclusion, message, nil)
		if err != nil {
			msg := fmt.Sprintf("report package vulnerability to github failed: %v", err)
			_, _ = io.WriteString(log, msg+"\n")
			common.LogError.Error(msg)
			return 0, err
		}
	}
	return len(data), nil
}
