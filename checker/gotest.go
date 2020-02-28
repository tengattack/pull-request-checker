package checker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/github"
	shellwords "github.com/mattn/go-shellwords"
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

// ReportTestResults reports the test results to github
func ReportTestResults(testName string, repoPath string, cmds []string, coveragePattern string, client *github.Client, gpull *github.PullRequest,
	ref GithubRef, targetURL string, log io.Writer) (string, error) {
	outputTitle := testName + " test"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t := github.Timestamp{Time: time.Now()}

	var checkRunID int64
	if ref.IsBranch() {
		err := ref.UpdateState(client, outputTitle, "pending", targetURL, "running")
		if err != nil {
			msg := fmt.Sprintf("Update commit state %s failed: %v", outputTitle, err)
			_, _ = io.WriteString(log, msg+"\n")
			LogError.Error(msg)
			// PASS
		}
	} else {
		checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
		if err != nil {
			msg := fmt.Sprintf("Creating %s check run failed: %v", outputTitle, err)
			_, _ = io.WriteString(log, msg+"\n")
			LogError.Error(msg)
			// PASS
		} else {
			checkRunID = checkRun.GetID()
		}
	}

	conclusion, reportMessage, outputSummary := testAndSaveCoverage(ctx, ref, testName, cmds,
		coveragePattern, repoPath, gpull, false, log)

	title := ""
	if coveragePattern == "" {
		title = conclusion
	} else {
		title = "coverage: " + reportMessage
	}
	if ref.IsBranch() {
		state := "success"
		if conclusion == "failure" {
			state = "error"
		}
		err := ref.UpdateState(client, outputTitle, state, targetURL, title)
		if err != nil {
			msg := fmt.Sprintf("Update commit state %s failed: %v", outputTitle, err)
			_, _ = io.WriteString(log, msg+"\n")
			LogError.Error(msg)
			// PASS
		}
	} else {
		if checkRunID == 0 {
			// create check run now if it failed before
			checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
			if err != nil {
				msg := fmt.Sprintf("Creating %s check run failed: %v", outputTitle, err)
				_, _ = io.WriteString(log, msg+"\n")
				LogError.Error(msg)
				// PASS
			} else {
				checkRunID = checkRun.GetID()
			}
		}

		if checkRunID != 0 {
			err := UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, title, "```\n"+outputSummary+"\n```", nil)
			if err != nil {
				LogError.Errorf("report test results to github failed: %v", err)
				// PASS
			}
		}
	}
	if conclusion == "failure" {
		err := &testNotPass{Title: outputTitle}
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
	pct, norm, err := util.ParseFloatPercent(coverage, 64)
	if err != nil {
		return coverage, 0, err
	}
	coverage = norm
	return coverage, pct, nil
}

func testAndSaveCoverage(ctx context.Context, ref GithubRef, testName string, cmds []string, coveragePattern string,
	repoPath string, gpull *github.PullRequest, breakOnFails bool, log io.Writer) (conclusion, reportMessage, outputSummary string) {
	parser := NewShellParser(repoPath, ref)

	_, _ = io.WriteString(log, fmt.Sprintf("Testing '%s'\n", testName))
	conclusion = "success"
	for _, cmd := range cmds {
		if cmd != "" {
			_, _ = io.WriteString(log, cmd+"\n")
			out := new(strings.Builder)
			tee := io.MultiWriter(log, out)
			errCmd := carry(ctx, parser, repoPath, cmd, tee)
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
			LogError.Error(msg)
			_, _ = io.WriteString(log, msg)
			// PASS
		}
		if err == nil || ref.IsBranch() {
			c := store.CommitsInfo{
				Owner:    ref.owner,
				Repo:     ref.repo,
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
				LogError.Error(msg)
				_, _ = io.WriteString(log, msg)
			}
		}

		outputSummary += ("Test coverage: " + percentage + "\n")
		reportMessage = percentage
	} else if coveragePattern == "" && ref.IsBranch() {
		pct := float64(-1)
		// saving build state with -1 coverage
		c := store.CommitsInfo{
			Owner:    ref.owner,
			Repo:     ref.repo,
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
			LogError.Error(msg)
			_, _ = io.WriteString(log, msg)
		}
	}
	_, _ = io.WriteString(log, "\n")
	return
}

// LogDivider provides the method to log stuff in parallel
type LogDivider struct {
	buffered bool
	log      io.Writer
	lm       *sync.Mutex
}

// NewLogDivider returns a new LogDivider
func NewLogDivider(buffered bool, log io.Writer) *LogDivider {
	lg := &LogDivider{
		buffered: buffered,
		log:      log,
	}
	if buffered {
		lg.lm = new(sync.Mutex)
	}
	return lg
}

// Log logs the given function f using LogDivider lg
func (lg *LogDivider) Log(f func(io.Writer)) {
	var w io.Writer
	if lg.buffered {
		w = new(bytes.Buffer)
	} else {
		w = lg.log
	}

	f(w)

	if lg.buffered {
		lg.lm.Lock()
		defer lg.lm.Unlock()
		lg.log.Write(w.(*bytes.Buffer).Bytes())
	}
}
