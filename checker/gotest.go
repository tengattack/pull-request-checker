package checker

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/google/go-github/github"
	shellwords "github.com/tengattack/go-shellwords"
)

type testResultProblemFound struct {
	TestTitle string
}

func (t *testResultProblemFound) Error() (s string) {
	if t != nil {
		return t.TestTitle
	}
	return
}

func carry(ctx context.Context, p *shellwords.Parser, repo, cmd string) (string, error) {
	words, err := p.Parse(cmd)
	if err != nil {
		return "", err
	}
	if len(words) < 1 {
		return "", errors.New("invalid command")
	}

	cmds := exec.CommandContext(ctx, words[0], words[1:]...)
	cmds.Dir = repo
	out, err := cmds.CombinedOutput()
	return string(out), err
}

// ReportTestResults reports the test results to github
func ReportTestResults(repo string, cmds []string, client *github.Client, gpull *github.PullRequest, outputTitle string, ref GithubRef, targetURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t := github.Timestamp{Time: time.Now()}

	checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
	if err != nil {
		LogError.Errorf("github create %s failed: %v", outputTitle, err)
		return err
	}
	checkRunID := checkRun.GetID()

	parser := shellwords.NewParser()
	parser.ParseEnv = true
	parser.ParseBacktick = true
	parser.Dir = repo

	var (
		conclusion    string
		outputSummary string
	)
	conclusion = "success"
	for _, cmd := range cmds {
		if cmd != "" {
			out, err := carry(ctx, parser, repo, cmd)
			outputSummary += ("\n" + out)
			if err != nil {
				conclusion = "failure"
				break
			}
		}
	}
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, outputTitle, "```\n"+outputSummary+"\n```", nil)
	if err != nil {
		LogError.Errorf("report test results to github failed: %v", err)
		return err
	}
	if conclusion == "failure" {
		err = &testResultProblemFound{TestTitle: outputTitle}
		return err
	}
	return nil
}
