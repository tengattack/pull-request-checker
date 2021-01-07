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
		execCommands := make([]*exec.Cmd, 0, len(pipelineCmds))
		for idx, cmd := range pipelineCmds {
			word, err := p.Parse(cmd)
			if err != nil {
				return err
			}
			if len(word) < 1 {
				return errors.New("invalid command")
			}
			exeCmd := exec.Command(word[0], word[1:]...)
			exeCmd.Dir = repo
			if idx == len(pipelineCmds)-1 {
				exeCmd.Stdout = log
				exeCmd.Stderr = log
			}
			if strings.Contains(cmd, ">") {
				// 最后一个 cmd 才可以重定向标准输出到文件
				if idx != len(pipelineCmds)-1 {
					return errors.New("invalid command")
				}
				cmds := strings.Split(cmd, " ")
				fileName := cmds[len(cmds)-1]
				outFile, _ := os.Create(path.Join(repo, fileName))
				exeCmd.Stdout = outFile
			}
			execCommands = append(execCommands, exeCmd)
		}
		return execute(execCommands...)
	}
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

func execute(stack ...*exec.Cmd) (err error) {
	pipeStack := make([]*io.PipeWriter, len(stack)-1)
	i := 0
	for ; i < len(stack)-1; i++ {
		stdinPipe, stdoutPipe := io.Pipe()
		stack[i].Stdout = stdoutPipe
		stack[i+1].Stdin = stdinPipe
		pipeStack[i] = stdoutPipe
	}

	return call(stack, pipeStack)
}

func call(stack []*exec.Cmd, pipes []*io.PipeWriter) (err error) {
	if stack[0].Process == nil {
		if err = stack[0].Start(); err != nil {
			return err
		}
	}
	if len(stack) > 1 {
		if err = stack[1].Start(); err != nil {
			return err
		}
		defer func() {
			if err == nil {
				pipes[0].Close()
				err = call(stack[1:], pipes[1:])
			}
		}()
	}
	return stack[0].Wait()
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
