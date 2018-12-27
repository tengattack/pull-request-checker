package checker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/google/go-github/github"
	"github.com/mattn/go-shellwords"
)

type testResultProblemFound struct {
}

func (*testResultProblemFound) Error() string {
	return "failure"
}

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
func ReportTestResults(repo, cmd, opt string, client *github.Client, gpull *GithubPull, outputTitle string, ref GithubRef, targetURL string) chan error {
	future := make(chan error, 1)
	go func() {
		defer func() {
			if info := recover(); info != nil {
				future <- fmt.Errorf("Panic: %v", info)
			}
			close(future)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		t := github.Timestamp{Time: time.Now()}

		checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
		if err != nil {
			LogError.Errorf("github create %s failed: %v", outputTitle, err)
			future <- err
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
			future <- err
			return
		}
		if conclusion == "failure" {
			future <- &testResultProblemFound{}
			return
		}
	}()
	return future
}
