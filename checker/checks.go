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
func CheckVulnerability(projectName, repoPath string) (bool, string, error) {
	secure := true
	securityMessage := ""

	scanner := riki.Scanner{ProjectName: projectName}
	gomod := filepath.Join(repoPath, "go.sum")
	if util.FileExists(gomod) {
		_, err := scanner.CheckPackages(common.Golang, gomod)
		if err != nil {
			return secure, securityMessage, err
		}
		scanner.WaitForQuery()
		ok, url, err := scanner.Query()
		if err != nil {
			return secure, securityMessage, err
		}
		if !ok {
			secure = false
			securityMessage += fmt.Sprintf("Found package vulnerabilities: %s\n", url)
		}
		return secure, securityMessage, nil
	}
	composer := filepath.Join(repoPath, "composer.lock")
	if util.FileExists(composer) {
		_, err := scanner.CheckPackages(common.PHP, composer)
		if err != nil {
			return secure, securityMessage, err
		}
		scanner.WaitForQuery()
		ok, url, err := scanner.Query()
		if err != nil {
			return secure, securityMessage, err
		}
		if !ok {
			secure = false
			securityMessage += fmt.Sprintf("Found package vulnerabilities: %s\n", url)
		}
		return secure, securityMessage, nil
	}
	return secure, securityMessage, nil
}

// VulnerabilityCheckRun checks and reports package vulnerabilities.
func VulnerabilityCheckRun(ctx context.Context, client *github.Client, gpull *github.PullRequest, ref GithubRef,
	repoPath string, targetURL string, log io.Writer) error {
	outputTitle := "vulnerability"
	checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
	if err != nil {
		msg := fmt.Sprintf("Creating %s check run failed: %v", outputTitle, err)
		_, _ = io.WriteString(log, msg+"\n")
		LogError.Error(msg)
		return err
	}
	ok, message, err := CheckVulnerability(ref.repo, repoPath)
	if err != nil {
		msg := fmt.Sprintf("checks package vulnerabilities failed: %v", err)
		_, _ = io.WriteString(log, msg+"\n")
		LogError.Error(msg)
		return err
	}
	checkRunID := checkRun.GetID()
	conclusion := "success"
	if !ok {
		conclusion = "failure"
	}
	t := github.Timestamp{Time: time.Now()}
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, conclusion, message, nil)
	if err != nil {
		msg := fmt.Sprintf("report package vulnerabilities to github failed: %v", err)
		_, _ = io.WriteString(log, msg+"\n")
		LogError.Error(msg)
		return err
	}
	return nil
}
