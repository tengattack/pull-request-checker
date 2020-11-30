package tester

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
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
	if strings.Contains(cmd, "|") {
		pipelineCmds := strings.Split(cmd, "|")
		if len(pipelineCmds) != 2 {
			return errors.New("invalid command")
		}
		word0, err := p.Parse(pipelineCmds[0])
		if err != nil {
			return err
		}
		if len(word0) < 1 {
			return errors.New("invalid command")
		}
		cmd0 := exec.Command(word0[0], word0[1:]...)
		cmd0.Dir = repo
		var outFile *os.File
		if strings.Contains(pipelineCmds[1], ">") {
			cmds := strings.Split(pipelineCmds[1], " ")
			fileName := cmds[len(cmds)-1]
			outFile, _ = os.Create(path.Join(repo, fileName))
		}
		word1, err := p.Parse(pipelineCmds[1])
		if err != nil {
			return err
		}
		cmd1 := exec.Command(word1[0], word1[1:]...)
		cmd1.Dir = repo
		pipelineSupport(cmd0, cmd1, log, outFile)
		return nil
	}
	words, err := p.Parse(cmd)
	if err != nil {
		return err
	}
	if len(words) < 1 {
		return errors.New("invalid command")
	}

	cmds := exec.Command(words[0], words[1:]...)
	cmds.Dir = repo
	cmds.Stdout = log
	cmds.Stderr = log

	return cmds.Run()
}

func pipelineSupport(c1, c2 *exec.Cmd, log io.Writer, outFile *os.File) {
	r, w := io.Pipe()
	c1.Stdout = w
	c2.Stdin = r
	c2.Stdout = log
	c2.Stderr = log
	if outFile != nil {
		c2.Stdout = outFile
	}
	c1.Start()
	c2.Start()
	c1.Wait()
	w.Close()
	c2.Wait()
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

func testAndSaveCoverage(ctx context.Context, ref common.GithubRef, testName string, cmds []string, coveragePattern string, deltaCoveragePattern string,
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
	if deltaCoveragePattern != "" && (conclusion == "success" || !breakOnFails) && ref.CheckType == common.CheckTypePRHead {
		deltaPercentage, _, err := parseCoverage(deltaCoveragePattern, outputSummary)
		if err != nil {
			msg := fmt.Sprintf("Failed to parse %s test coverage: %v\n", testName, err)
			common.LogError.Error(msg)
			_, _ = io.WriteString(log, msg)
			// PASS
		}
		outputSummary += ("Delta Test coverage: " + deltaPercentage + "\n")
		if reportMessage != "" {
			reportMessage = fmt.Sprintf("%s, %s", reportMessage, deltaPercentage)
		} else {
			reportMessage = deltaPercentage
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
