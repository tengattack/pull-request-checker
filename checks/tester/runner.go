package tester

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/google/go-github/github"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/store"
	"github.com/tengattack/unified-ci/util"
)

type Result struct {
	Conclusion    string
	ReportMessage string
	OutputSummary string
}

type Runner interface {
	Run(ctx context.Context, testName string, testConfig util.TestsConfig) (*Result, error)
}

func isEmptyTest(cmds []string) bool {
	empty := true
	for _, c := range cmds {
		if c != "" {
			empty = false
		}
	}
	return empty
}

func RunTests(ctx context.Context, tests map[string]util.TestsConfig, t Runner, coverageMap *sync.Map) (failedTests, passedTests, errTests int) {
	maxPendingTests := common.Conf.Concurrency.Test
	if maxPendingTests < 1 {
		maxPendingTests = 1
	}
	pendingTests := make(chan int, maxPendingTests)

	var (
		wg          sync.WaitGroup
		errCount    int64
		failedCount int64
		passedCount int64
	)
	for k, v := range tests {
		testName := k
		testConfig := v

		if isEmptyTest(testConfig.Cmds) {
			continue
		}

		pendingTests <- 0
		wg.Add(1)
		go func() {
			defer func() {
				if info := recover(); info != nil {
					atomic.AddInt64(&errCount, 1)
				}
				wg.Done()
				<-pendingTests
			}()
			result, err := t.Run(ctx, testName, testConfig)
			if testConfig.Coverage != "" {
				coverageMap.Store(testName, result.ReportMessage)
			}
			if err != nil {
				if _, ok := err.(*testNotPass); ok {
					atomic.AddInt64(&failedCount, 1)
				} else {
					atomic.AddInt64(&errCount, 1)
				}
			} else {
				atomic.AddInt64(&passedCount, 1)
			}
		}()
	}

	wg.Wait()
	failedTests = int(failedCount)
	passedTests = int(passedCount)
	errTests = int(errCount)
	return
}

func LoadBaseFromStore(ref common.GithubRef, baseSHA string, tests map[string]util.TestsConfig,
	log io.Writer) ([]store.CommitsInfo, map[string]util.TestsConfig) {
	baseSavedRecords, err := store.ListCommitsInfo(ref.Owner, ref.RepoName, baseSHA)
	if err != nil {
		msg := fmt.Sprintf("Failed to load base info: %v\n", err)
		common.LogError.Error(msg)
		io.WriteString(log, msg)
		// PASS
	}

	baseTestsNeedToRun := make(map[string]util.TestsConfig)
	for testName, testCfg := range tests {
		found := false
		if testCfg.Coverage == "" {
			// no need to run in base as it has no coverage requirements
			continue
		}
		for _, v := range baseSavedRecords {
			if testName == v.Test {
				found = true
				break
			}
		}
		if !found {
			baseTestsNeedToRun[testName] = testCfg
		}
	}
	io.WriteString(log,
		fmt.Sprintf("baseSavedRecords: %d, baseTestsNeedToRun: %d\n\n", len(baseSavedRecords), len(baseTestsNeedToRun)))
	return baseSavedRecords, baseTestsNeedToRun
}

func FindBaseCoverage(ctx context.Context, baseSavedRecords []store.CommitsInfo, baseTestsNeedToRun map[string]util.TestsConfig, repoPath string,
	baseSHA string, gpull *github.PullRequest, ref common.GithubRef, log io.Writer, baseCoverage *sync.Map) error {
	for _, v := range baseSavedRecords {
		if v.Coverage == nil {
			baseCoverage.Store(v.Test, "nil")
		} else {
			baseCoverage.Store(v.Test, util.FormatFloatPercent(*v.Coverage))
		}
	}

	parser := util.NewShellParser(repoPath, ref)
	words, err := parser.Parse(common.Conf.Core.GitCommand)
	if err != nil {
		err = fmt.Errorf("parse git command error: %v", err)
		return err
	}

	if len(baseTestsNeedToRun) > 0 {
		io.WriteString(log, "$ git checkout -f "+baseSHA+"\n")
		gitCmds := make([]string, len(words))
		copy(gitCmds, words)
		gitCmds = append(gitCmds, "checkout", "-f", baseSHA)
		cmd := exec.Command(gitCmds[0], gitCmds[1:]...)
		cmd.Dir = repoPath
		cmd.Stdout = log
		cmd.Stderr = log
		err := cmd.Run()
		if err != nil {
			msg := fmt.Sprintf("Failed to checkout to base: %v\n", err)
			common.LogError.Error(msg)
			io.WriteString(log, msg)
			return err
		}

		t := &baseTest{
			Ref:      ref,
			BaseSHA:  baseSHA,
			RepoPath: repoPath,
			Pull:     gpull,
		}
		t.LogDivider = util.NewLogDivider(len(baseTestsNeedToRun) > 1, log)
		RunTests(ctx, baseTestsNeedToRun, t, baseCoverage)

		io.WriteString(log, "$ git checkout -f "+ref.Sha+"\n")
		gitCmds = make([]string, len(words))
		copy(gitCmds, words)
		gitCmds = append(gitCmds, "checkout", "-f", ref.Sha)
		cmd = exec.Command(gitCmds[0], gitCmds[1:]...)
		cmd.Dir = repoPath
		cmd.Stdout = log
		cmd.Stderr = log
		err = cmd.Run()
		if err != nil {
			msg := fmt.Sprintf("Failed to checkout back: %v\n", err)
			common.LogError.Error(msg)
			io.WriteString(log, msg)
			return err
		}
	}
	return nil
}

type baseTest struct {
	*util.LogDivider

	Ref      common.GithubRef
	BaseSHA  string
	RepoPath string
	Pull     *github.PullRequest
}

func (t *baseTest) Run(ctx context.Context, testName string, testConfig util.TestsConfig) (*Result, error) {
	var result *Result
	t.Log(func(w io.Writer) {
		ref := t.Ref
		ref.Sha = t.BaseSHA
		if ref.CheckType == common.CheckTypePRHead {
			ref.CheckType = common.CheckTypePRBase
		}

		result = testAndSaveCoverage(ctx, ref,
			testName, testConfig.Cmds, testConfig.Coverage, t.RepoPath, t.Pull, true, w)
	})
	return result, nil
}

type HeadTest struct {
	*util.LogDivider

	RepoPath  string
	Client    *github.Client
	Pull      *github.PullRequest
	Ref       common.GithubRef
	TargetURL string
}

func (t *HeadTest) Run(ctx context.Context, testName string, testConfig util.TestsConfig) (result *Result, err error) {
	t.Log(func(w io.Writer) {
		result = testAndSaveCoverage(ctx, t.Ref,
			testName, testConfig.Cmds, testConfig.Coverage, t.RepoPath, t.Pull, false, w)
		if result.Conclusion == "failure" {
			err = &testNotPass{Title: ""}
		}
	})
	return
}
