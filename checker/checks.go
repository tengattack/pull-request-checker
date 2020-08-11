package checker

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/go-github/github"
	vul "github.com/tengattack/unified-ci/checks/vulnerability"
	vulcommon "github.com/tengattack/unified-ci/checks/vulnerability/common"
	"github.com/tengattack/unified-ci/util"
)

// CheckVulnerability checks the package vulnerability of repo
func CheckVulnerability(projectName, repoPath, commitID, context string) (result []vulcommon.Data, err error) {
	var lang []vulcommon.Language
	scanner := vul.NewScanner(projectName, Conf.Vulnerability)
	scanner.SetCommitID(commitID)
	scanner.SetContext(context)

	gomod := filepath.Join(repoPath, "go.sum")
	if util.FileExists(gomod) {
		_, err := scanner.CheckPackages(vulcommon.Golang, gomod)
		if err != nil {
			return nil, err
		}
		lang = append(lang, vulcommon.Golang)
	}
	composer := filepath.Join(repoPath, "composer.lock")
	if util.FileExists(composer) {
		_, err := scanner.CheckPackages(vulcommon.PHP, composer)
		if err != nil {
			return nil, err
		}
		lang = append(lang, vulcommon.PHP)
	}
	nodePackage := filepath.Join(repoPath, "package.json")
	if util.FileExists(nodePackage) {
		_, err := scanner.CheckPackages(vulcommon.NodeJS, nodePackage)
		if err != nil {
			return nil, err
		}
		lang = append(lang, vulcommon.NodeJS)
	}

	if len(lang) > 0 {
		scanner.WaitForQuery()
		for _, v := range lang {
			data, err := scanner.Query(v)
			if err != nil {
				return nil, err
			}
			result = append(result, data...)
		}
	}
	return result, nil
}

// VulnerabilityCheckRun checks and reports package vulnerability.
func VulnerabilityCheckRun(ctx context.Context, client *github.Client, gpull *github.PullRequest, ref GithubRef,
	repoPath string, targetURL string, log io.Writer) (int, error) {
	const checkName = "vulnerability"
	var checkRunID int64

	if ref.IsBranch() {
		err := ref.UpdateState(client, checkName, "pending", targetURL, "running")
		if err != nil {
			msg := fmt.Sprintf("Update commit state %s failed: %v", checkName, err)
			_, _ = io.WriteString(log, msg+"\n")
			LogError.Error(msg)
			// PASS
		}
	} else {
		checkRun, err := CreateCheckRun(ctx, client, gpull, checkName, ref, targetURL)
		if err != nil {
			msg := fmt.Sprintf("Creating %s check run failed: %v", checkName, err)
			_, _ = io.WriteString(log, msg+"\n")
			LogError.Error(msg)
			// PASS
		} else {
			checkRunID = checkRun.GetID()
		}
	}

	data, err := CheckVulnerability(ref.repo, repoPath, ref.Sha, ref.checkRef)
	if err != nil {
		msg := fmt.Sprintf("checks package vulnerability failed: %v", err)
		_, _ = io.WriteString(log, msg+"\n")
		LogError.Error(msg)
		if ref.IsBranch() {
			err := ref.UpdateState(client, checkName, "failure", targetURL, "")
			if err != nil {
				msg := fmt.Sprintf("Update commit state %s failed: %v", checkName, err)
				_, _ = io.WriteString(log, msg+"\n")
				LogError.Error(msg)
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
			LogError.Error(msg)
			// PASS
		}
	} else {
		if checkRunID == 0 {
			checkRun, err := CreateCheckRun(ctx, client, gpull, checkName, ref, targetURL)
			if err != nil {
				msg := fmt.Sprintf("Creating %s check run failed: %v", checkName, err)
				_, _ = io.WriteString(log, msg+"\n")
				LogError.Error(msg)
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
			LogError.Error(msg)
			return 0, err
		}
	}
	return len(data), nil
}
