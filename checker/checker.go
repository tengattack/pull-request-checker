package checker

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"github.com/tengattack/unified-ci/checks/tester"
	"github.com/tengattack/unified-ci/checks/vulnerability"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/util"
)

type testCheckRun struct {
	*util.LogDivider

	runner *tester.HeadTest
}

func (t *testCheckRun) Run(ctx context.Context, testName string, testConfig util.TestsConfig) (result *tester.Result, err error) {
	t.Log(func(w io.Writer) {
		t.runner.LogDivider = util.NewLogDivider(false, w)
		outputTitle := testName + " test"

		client := t.runner.Client
		gpull := t.runner.Pull
		ref, targetURL := t.runner.Ref, t.runner.TargetURL

		var checkRunID int64
		if ref.IsBranch() {
			err := ref.UpdateState(client, outputTitle, "pending", targetURL, "running")
			if err != nil {
				msg := fmt.Sprintf("Update commit state %s failed: %v", outputTitle, err)
				_, _ = io.WriteString(w, msg+"\n")
				common.LogError.Error(msg)
				// PASS
			}
		} else {
			checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
			if err != nil {
				msg := fmt.Sprintf("Creating %s check run failed: %v", outputTitle, err)
				_, _ = io.WriteString(w, msg+"\n")
				common.LogError.Error(msg)
				// PASS
			} else {
				checkRunID = checkRun.GetID()
			}
		}

		result, err = t.runner.Run(ctx, testName, testConfig)

		title := ""
		if testConfig.Coverage == "" {
			title = result.Conclusion
		} else {
			title = "coverage: " + result.ReportMessage
		}
		if ref.IsBranch() {
			state := "success"
			if result.Conclusion == "failure" {
				state = "error"
			}
			err := ref.UpdateState(client, outputTitle, state, targetURL, title)
			if err != nil {
				msg := fmt.Sprintf("Update commit state %s failed: %v", outputTitle, err)
				_, _ = io.WriteString(w, msg+"\n")
				common.LogError.Error(msg)
				// PASS
			}
		} else {
			if checkRunID == 0 {
				// create check run now if it failed before
				checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
				if err != nil {
					msg := fmt.Sprintf("Creating %s check run failed: %v", outputTitle, err)
					_, _ = io.WriteString(w, msg+"\n")
					common.LogError.Error(msg)
					// PASS
				} else {
					checkRunID = checkRun.GetID()
				}
			}

			if checkRunID != 0 {
				ts := github.Timestamp{Time: time.Now()}
				err := UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, result.Conclusion, ts,
					title, "```\n"+result.OutputSummary+"\n```", nil)
				if err != nil {
					common.LogError.Errorf("report test results to github failed: %v", err)
					// PASS
				}
			}
		}
	})
	return
}

// TestCheckRun run tests and report the test results to github
func TestCheckRun(ctx context.Context, repoPath string, client *github.Client, gpull *github.PullRequest,
	ref common.GithubRef, targetURL string,
	tests map[string]util.TestsConfig, coverageMap *sync.Map, log io.Writer) (failedTests, passedTests, errTests int, err error) {

	runner := &tester.HeadTest{
		RepoPath:  repoPath,
		Client:    client,
		Pull:      gpull,
		Ref:       ref,
		TargetURL: targetURL,
	}
	t := &testCheckRun{runner: runner}
	t.LogDivider = util.NewLogDivider(len(tests) > 1, log)
	failedTests, passedTests, errTests = tester.RunTests(ctx, tests, t, coverageMap)

	return
}

// VulnerabilityCheckRun checks and reports package vulnerability.
func VulnerabilityCheckRun(ctx context.Context, client *github.Client, gpull *github.PullRequest, ref common.GithubRef,
	repoPath string, targetURL string, log io.Writer) (int, error) {
	const checkName = "vulnerability"
	var checkRunID int64

	if ref.IsBranch() {
		err := ref.UpdateState(client, checkName, "pending", targetURL, "running")
		if err != nil {
			msg := fmt.Sprintf("Update commit state %s failed: %v", checkName, err)
			_, _ = io.WriteString(log, msg+"\n")
			common.LogError.Error(msg)
			// PASS
		}
	} else {
		checkRun, err := CreateCheckRun(ctx, client, gpull, checkName, ref, targetURL)
		if err != nil {
			msg := fmt.Sprintf("Creating %s check run failed: %v", checkName, err)
			_, _ = io.WriteString(log, msg+"\n")
			common.LogError.Error(msg)
			// PASS
		} else {
			checkRunID = checkRun.GetID()
		}
	}

	data, err := vulnerability.CheckVulnerability(ref.RepoName, repoPath, ref.Sha, ref.CheckRef)
	if err != nil {
		msg := fmt.Sprintf("checks package vulnerability failed: %v", err)
		_, _ = io.WriteString(log, msg+"\n")
		common.LogError.Error(msg)
		if ref.IsBranch() {
			err := ref.UpdateState(client, checkName, "failure", targetURL, "")
			if err != nil {
				msg := fmt.Sprintf("Update commit state %s failed: %v", checkName, err)
				_, _ = io.WriteString(log, msg+"\n")
				common.LogError.Error(msg)
				// PASS
			}
		} else {
			if checkRunID != 0 {
				UpdateCheckRunWithError(ctx, client, gpull, checkRunID, checkName, checkName, err)
			}
		}
		return 0, err
	}

	if ref.IsBranch() {
		state := "success"
		title := "no vulnerabilities"
		if len(data) > 0 {
			state = "error"
			title = fmt.Sprintf("%d problem(s) found.", len(data))
		}
		err := ref.UpdateState(client, checkName, state, targetURL, title)
		if err != nil {
			msg := fmt.Sprintf("Update commit state %s failed: %v", checkName, err)
			_, _ = io.WriteString(log, msg+"\n")
			common.LogError.Error(msg)
			// PASS
		}
	} else {
		if checkRunID == 0 {
			checkRun, err := CreateCheckRun(ctx, client, gpull, checkName, ref, targetURL)
			if err != nil {
				msg := fmt.Sprintf("Creating %s check run failed: %v", checkName, err)
				_, _ = io.WriteString(log, msg+"\n")
				common.LogError.Error(msg)
				return 0, err
			}
			checkRunID = checkRun.GetID()
		}

		conclusion := "success"
		message := "no vulnerabilities"
		if len(data) > 0 {
			conclusion = "failure"
			message = data[0].MDTitle()
			for _, v := range data {
				message += v.MDTableRow()
			}
		}

		t := github.Timestamp{Time: time.Now()}
		err = UpdateCheckRun(ctx, client, gpull, checkRunID, checkName, conclusion, t, conclusion, message, nil)
		if err != nil {
			msg := fmt.Sprintf("report package vulnerability to github failed: %v", err)
			_, _ = io.WriteString(log, msg+"\n")
			common.LogError.Error(msg)
			return 0, err
		}
	}
	return len(data), nil
}
