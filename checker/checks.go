package checker

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/go-github/github"
	"github.com/tengattack/unified-ci/checks/vulnerability/common"
	"github.com/tengattack/unified-ci/checks/vulnerability/riki"
	"github.com/tengattack/unified-ci/util"
)

// CheckVulnerability checks the package vulnerability of repo
func CheckVulnerability(projectName, repoPath string) (bool, []riki.Data, error) {
	scanner := riki.Scanner{ProjectName: projectName}
	gomod := filepath.Join(repoPath, "go.sum")
	if util.FileExists(gomod) {
		_, err := scanner.CheckPackages(common.Golang, gomod)
		if err != nil {
			return true, nil, err
		}
		scanner.WaitForQuery()
		return scanner.Query(common.Golang)
	}
	composer := filepath.Join(repoPath, "composer.lock")
	if util.FileExists(composer) {
		_, err := scanner.CheckPackages(common.PHP, composer)
		if err != nil {
			return true, nil, err
		}
		scanner.WaitForQuery()
		return scanner.Query(common.PHP)
	}
	nodePackage := filepath.Join(repoPath, "package.json")
	if util.FileExists(nodePackage) {
		_, err := scanner.CheckPackages(common.NodeJS, nodePackage)
		if err != nil {
			return true, nil, err
		}
		scanner.WaitForQuery()
		return scanner.Query(common.NodeJS)
	}
	return true, nil, nil
}

// VulnerabilityCheckRun checks and reports package vulnerability.
func VulnerabilityCheckRun(ctx context.Context, client *github.Client, gpull *github.PullRequest, ref GithubRef,
	repoPath string, targetURL string, log io.Writer) (int, error) {
	const checkName = "vulnerability"
	var checkRunID int64
	checkRun, err := CreateCheckRun(ctx, client, gpull, checkName, ref, targetURL)
	if err != nil {
		msg := fmt.Sprintf("Creating %s check run failed: %v", checkName, err)
		_, _ = io.WriteString(log, msg+"\n")
		LogError.Error(msg)
		// PASS
	} else {
		checkRunID = checkRun.GetID()
	}

	ok, data, err := CheckVulnerability(ref.repo, repoPath)
	if err != nil {
		msg := fmt.Sprintf("checks package vulnerability failed: %v", err)
		_, _ = io.WriteString(log, msg+"\n")
		LogError.Error(msg)
		if checkRunID != 0 {
			UpdateCheckRunWithError(ctx, client, gpull, checkRunID, checkName, checkName, err)
		}
		return 0, err
	}

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
	if !ok {
		conclusion = "failure"
	}
	t := github.Timestamp{Time: time.Now()}
	message := riki.Data{}.MDTitle()
	for _, v := range data {
		message += v.MDTableRow()
	}
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, checkName, conclusion, t, conclusion, message, nil)
	if err != nil {
		msg := fmt.Sprintf("report package vulnerability to github failed: %v", err)
		_, _ = io.WriteString(log, msg+"\n")
		LogError.Error(msg)
		return 0, err
	}
	return len(data), nil
}
