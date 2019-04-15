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
func ReportTestResults(repo string, cmds []string, coveragePattern string, client *github.Client, gpull *github.PullRequest,
	outputTitle string, ref GithubRef, targetURL string) (addMsg string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t := github.Timestamp{Time: time.Now()}

	checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
	if err != nil {
		LogError.Errorf("github create %s failed: %v", outputTitle, err)
		return "", err
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
			out, errCmd := carry(ctx, parser, repo, cmd)
			outputSummary += ("\n" + out)
			if errCmd != nil {
				conclusion = "failure"
				break
			}
		}
	}
	if conclusion == "success" && coveragePattern != "" {
		result, _ := parseCoverage(coveragePattern, outputSummary)
		outputSummary += ("\n" + "Test coverage: " + result)
		addMsg += (outputTitle + " coverage: " + result)
	}
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, outputTitle, "```\n"+outputSummary+"\n```", nil)
	if err != nil {
		LogError.Errorf("report test results to github failed: %v", err)
		return "", err
	}
	if conclusion == "failure" {
		err = &testNotPass{Title: outputTitle}
		return "", err
	}
	return addMsg, nil
}

func parseCoverage(pattern, output string) (string, error) {
	coverage := "unknown"
	r, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	match := r.FindStringSubmatch(output)
	if len(match) > 1 {
		coverage = match[1]
	}
	return coverage, nil
}
