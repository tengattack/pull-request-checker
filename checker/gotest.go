package checker

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/mattn/go-shellwords"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

// Gotest runs "go test ./..." in the repo directory
func Gotest(ctx context.Context, command string, repo string) (string, error) {
	words, err := shellwords.Parse(command)
	if err != nil {
		return "", err
	}
	if len(words) < 2 {
		return "", errors.New(`Gotest command should consist of at least 2 words, e.g. "go test"`)
	}

	words = append(words, "./...")

	cmd := exec.CommandContext(ctx, words[0], words[1:]...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ReportGotest reports the go test result to github
func ReportGotest(repo string, diffs []*diff.FileDiff, client *github.Client, gpull *GithubPull, ref GithubRef, targetURL string) {
	if !isGoFileChanged(diffs) {
		LogAccess.Info("No go files are needed to check")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	t := github.Timestamp{Time: time.Now()}

	outputTitle := "gotest"
	checkRun, err := CreateCheckRun(ctx, client, outputTitle, gpull, ref, targetURL)
	if err != nil {
		LogError.Errorf("github create gotest failed: %v", err)
		return
	}
	checkRunID := checkRun.GetID()

	outputSummary, err := Gotest(ctx, Conf.Core.Gotest, repo)
	if err != nil {
		err = UpdateCheckRun(ctx, client, checkRunID, outputTitle, gpull, "failure", t, outputTitle, outputSummary, nil)
	}
	err = UpdateCheckRun(ctx, client, checkRunID, outputTitle, gpull, "success", t, outputTitle, outputSummary, nil)
	if err != nil {
		UpdateCheckRunWithError(ctx, client, checkRunID, outputTitle, outputTitle, err, gpull)
	}
}

func isGoFileChanged(diffs []*diff.FileDiff) bool {
	for _, d := range diffs {
		newName, err := strconv.Unquote(d.NewName)
		if err != nil {
			newName = d.NewName
		}
		if strings.HasPrefix(newName, "b/") {
			fileName := newName[2:]
			if strings.HasSuffix(fileName, ".go") {
				return true
			}
		}
	}
	return false
}
