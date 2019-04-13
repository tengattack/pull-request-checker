package checker

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"time"

	"github.com/google/go-github/github"
	shellwords "github.com/tengattack/go-shellwords"
)

type testNotPass struct {
	Title string
}

func (t *testNotPass) Error() (s string) {
	if t != nil {
		return t.Title
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
func ReportTestResults(repo string, tasks []testTask, client *github.Client, gpull *github.PullRequest, outputTitle string, ref GithubRef, targetURL string) error {
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
	for _, task := range tasks {
		if task != nil {
			cmd, _ := task["cmd"]
			if cmd != "" {
				out, errCmd := carry(ctx, parser, repo, cmd)
				coverage, _ := task["coverage"]
				if coverage != "" {
					cover := "unknown"
					r, err := regexp.Compile(coverage)
					if err == nil {
						match := r.FindStringSubmatch(out)
						if len(match) > 1 {
							cover = match[1]
						}
					}
					outputSummary += ("\n" + "Test coverage: " + cover)
				} else {
					outputSummary += ("\n" + out)
				}
				if errCmd != nil {
					conclusion = "failure"
					break
				}
			}
		}
	}
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, outputTitle, "```\n"+outputSummary+"\n```", nil)
	if err != nil {
		LogError.Errorf("report test results to github failed: %v", err)
		return err
	}
	if conclusion == "failure" {
		err = &testNotPass{Title: outputTitle}
		return err
	}
	return nil
}
