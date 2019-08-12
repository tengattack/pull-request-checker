package checker

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	ref GithubRef, targetURL string, log io.Writer) (string, error) {
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

	conclusion, reportMessage, outputSummary := testAndSaveCoverage(ctx, ref.owner, ref.repo, ref.Sha, testName, cmds,
		coveragePattern, repoPath, gpull, false, log)
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, "coverage: "+reportMessage, "```\n"+outputSummary+"\n```", nil)
	if err != nil {
		LogError.Errorf("report test results to github failed: %v", err)
		// PASS
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

func testAndSaveCoverage(ctx context.Context, owner, repo, sha string, testName string, cmds []string, coveragePattern string,
	repoPath string, gpull *github.PullRequest, breakOnFails bool, log io.Writer) (conclusion, reportMessage, outputSummary string) {
	parser := NewShellParser(repoPath)

	io.WriteString(log, fmt.Sprintf("Testing '%s'\n", testName))
	conclusion = "success"
	for _, cmd := range cmds {
		if cmd != "" {
			out, errCmd := carry(ctx, parser, repoPath, cmd)
			msg := cmd + "\n" + out + "\n"
			if errCmd != nil {
				msg += errCmd.Error() + "\n"
			}

			io.WriteString(log, msg)
			outputSummary += msg
			if errCmd != nil {
				conclusion = "failure"
				if breakOnFails {
					break
				}
			}
		}
	}
	// get test coverage even if the conclusion is failure when ignoring the failed tests
	if coveragePattern != "" && (conclusion == "success" || !breakOnFails) {
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
				msg := fmt.Sprintf("Error: %v. Failed to save %v\n", err, c)
				outputSummary += msg
				LogError.Error(msg)
				io.WriteString(log, msg)
			}
		} else {
			msg := fmt.Sprintf("Failed to parse '%s': %v\n", percentage, err)
			LogError.Error(msg)
			io.WriteString(log, msg)
			// PASS
		}

		outputSummary += ("Test coverage: " + percentage + "\n")
		reportMessage = percentage
	}
	io.WriteString(log, "\n")
	return
}
