package checker

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/google/go-github/github"
	"github.com/mattn/go-shellwords"
)

func carry(ctx context.Context, repo, cmd, opt string) (string, error) {
	words, err := shellwords.Parse(cmd)
	if err != nil {
		return "", err
	}
	if opt != "" {
		words = append(words, opt)
	}
	if len(words) < 1 {
		return "", errors.New("cmd + opt should consist of at least 1 word")
	}

	cmds := exec.CommandContext(ctx, words[0], words[1:]...)
	cmds.Dir = repo
	out, err := cmds.CombinedOutput()
	return string(out), err
}

// ReportTestResults reports the test results to github
func ReportTestResults(repo, cmd, opt string, client *github.Client, gpull *GithubPull, outputTitle string, ref GithubRef, targetURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t := github.Timestamp{Time: time.Now()}

	checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
	if err != nil {
		LogError.Errorf("github create %s failed: %v", outputTitle, err)
		return
	}
	checkRunID := checkRun.GetID()

	outputSummary, err := carry(ctx, repo, cmd, opt)
	var conclusion string
	if err != nil {
		conclusion = "failure"
	} else {
		conclusion = "success"
	}
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, outputTitle, "```\n"+outputSummary+"\n```", nil)
	if err != nil {
		LogError.Errorf("report test results to github failed: %v", err)
	}
}
