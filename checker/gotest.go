package checker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"time"

	"github.com/google/go-github/github"
	shellwords "github.com/tengattack/go-shellwords"
	"github.com/tengattack/unified-ci/store"
	"github.com/tengattack/unified-ci/util"
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
func ReportTestResults(testName string, repoPath string, cmds []string, coveragePattern string, client *github.Client, gpull *github.PullRequest,
	ref GithubRef, targetURL string) (string, error) {
	outputTitle := testName + " test"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t := github.Timestamp{Time: time.Now()}

	checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
	if err != nil {
		LogError.Errorf("github create %s failed: %v", outputTitle, err)
		return "", err
	}
	checkRunID := checkRun.GetID()

	conclusion, reportMessage, outputSummary := launchCommands(ctx, ref.owner, ref.repo, ref.Sha, testName, cmds,
		coveragePattern, repoPath, gpull, false)
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, "coverage: "+reportMessage, "```\n"+outputSummary+"\n```", nil)
	if err != nil {
		LogError.Errorf("report test results to github failed: %v", err)
		return reportMessage, err
	}
	if conclusion == "failure" {
		err = &testNotPass{Title: outputTitle}
		return reportMessage, err
	}
	return reportMessage, nil
}

func parseCoverage(pattern, output string) (string, float64, error) {
	coverage := "unknown"
	r, err := regexp.Compile(pattern)
	if err != nil {
		return "error", 0, err
	}
	match := r.FindStringSubmatch(output)
	if len(match) > 1 {
		coverage = match[1]
	}
	pct, err := util.ParseFloatPercent(coverage, 64)
	if err != nil {
		return coverage, 0, err
	}
	return coverage, pct, nil
}

func launchCommands(ctx context.Context, owner, repo, sha string, testName string, cmds []string, coveragePattern string,
	repoPath string, gpull *github.PullRequest, breakOnFails bool) (conclusion, reportMessage, outputSummary string) {
	parser := shellwords.NewParser()
	parser.ParseEnv = true
	parser.ParseBacktick = true
	parser.Dir = repoPath

	conclusion = "success"
	for _, cmd := range cmds {
		if cmd != "" {
			out, errCmd := carry(ctx, parser, repoPath, cmd)
			outputSummary += ("\n" + out)
			if errCmd != nil {
				conclusion = "failure"
				if breakOnFails {
					break
				}
			}
		}
	}
	// get test coverage even if the conclusion is failure when ignoring the failed tests
	if coveragePattern != "" && (!breakOnFails || conclusion == "success") {
		percentage, pct, err := parseCoverage(coveragePattern, outputSummary)
		if err == nil {
			c := store.CommitsInfo{
				Owner:    owner,
				Repo:     repo,
				Sha:      sha,
				Author:   gpull.GetHead().GetUser().GetLogin(),
				Test:     testName,
				Coverage: &pct,
			}
			err := c.Save()
			if err != nil {
				msg := fmt.Sprintf("\nError: %v. Failed to save %v", err, c)
				outputSummary += msg
				LogError.Errorf(msg)
			}
		} else {
			LogError.Errorf("Failed to parse '%s': %v", percentage, err)
			// PASS
		}

		outputSummary += ("\nTest coverage: " + percentage)
		reportMessage = percentage
	}
	return
}
