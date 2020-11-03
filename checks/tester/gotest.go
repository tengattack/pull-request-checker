package tester

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/tengattack/unified-ci/common"
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

func carry(ctx context.Context, p *shellwords.Parser, repo, cmd string, log io.Writer) error {
	words, err := p.Parse(cmd)
	if err != nil {
		return err
	}
	if len(words) < 1 {
		return errors.New("invalid command")
	}

	cmds := exec.CommandContext(ctx, words[0], words[1:]...)
	cmds.Dir = repo
	cmds.Stdout = log
	cmds.Stderr = log

	return cmds.Run()
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
	pct, norm, err := util.ParseFloatPercent(coverage, 64)
	if err != nil {
		return coverage, 0, err
	}
	coverage = norm
	return coverage, pct, nil
}

func testAndSaveCoverage(ctx context.Context, ref common.GithubRef, testName string, cmds []string, coveragePattern string,
	repoPath string, gpull *github.PullRequest, breakOnFails bool, log io.Writer) (result *Result) {
	var reportMessage, outputSummary string
	parser := util.NewShellParser(repoPath, ref)

	_, _ = io.WriteString(log, fmt.Sprintf("Testing '%s'\n", testName))
	conclusion := "success"
	for _, cmd := range cmds {
		if cmd != "" {
			_, _ = io.WriteString(log, cmd+"\n")
			out := new(strings.Builder)
			errCmd := carry(ctx, parser, repoPath, cmd, io.MultiWriter(log, out))
			outputSummary += cmd + "\n" + out.String() + "\n"
			if errCmd != nil {
				errMsg := errCmd.Error() + "\n"
				outputSummary += errMsg
				_, _ = io.WriteString(log, errMsg)
			}

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
		if err != nil {
			msg := fmt.Sprintf("Failed to parse %s test coverage: %v\n", testName, err)
			common.LogError.Error(msg)
			_, _ = io.WriteString(log, msg)
			// PASS
		}
		if err == nil || ref.IsBranch() {
			c := store.CommitsInfo{
				Owner:    ref.Owner,
				Repo:     ref.RepoName,
				Sha:      ref.Sha,
				Author:   gpull.GetHead().GetUser().GetLogin(),
				Test:     testName,
				Coverage: &pct,
			}
			if conclusion == "success" {
				c.Passing = 1
			}
			if ref.IsBranch() {
				// always save for tree test check
				c.Status = 1
				if err != nil {
					c.Coverage = nil
				}
			}
			err := c.Save()
			if err != nil {
				msg := fmt.Sprintf("Error: %v. Failed to save %v\n", err, c)
				outputSummary += msg
				common.LogError.Error(msg)
				_, _ = io.WriteString(log, msg)
			}
		}

		outputSummary += ("Test coverage: " + percentage + "\n")
		reportMessage = percentage
	} else if coveragePattern == "" && ref.IsBranch() {
		pct := float64(-1)
		// saving build state with -1 coverage
		c := store.CommitsInfo{
			Owner:    ref.Owner,
			Repo:     ref.RepoName,
			Sha:      ref.Sha,
			Author:   gpull.GetHead().GetUser().GetLogin(),
			Test:     testName,
			Coverage: &pct,
			Passing:  0,
			Status:   1,
		}
		if conclusion == "success" {
			c.Passing = 1
		}
		err := c.Save()
		if err != nil {
			msg := fmt.Sprintf("Error: %v. Failed to save %v\n", err, c)
			common.LogError.Error(msg)
			_, _ = io.WriteString(log, msg)
		}
	}
	_, _ = io.WriteString(log, "\n")
	result = &Result{
		Conclusion:    conclusion,
		ReportMessage: reportMessage,
		OutputSummary: outputSummary,
	}
	return
}
