package checker

import (
	"context"
	"os/exec"
	"time"

	"github.com/google/go-github/github"

	"github.com/mattn/go-shellwords"
)

// Gotest runs "go test ./..." in the repo directory
func Gotest(ctx context.Context, repo string) (string, error) {
	flags, err := shellwords.Parse(Conf.Core.Gotest)
	if err != nil {
		return "", err
	}

	options := []string{"test", "./..."}
	options = append(options, flags...)

	cmd := exec.CommandContext(ctx, "go", options...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ReportGotest reports the go test result to github
func ReportGotest(repo string, client *github.Client, gpull *GithubPull, ref GithubRef, targetURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	languages, _, err := client.Repositories.ListLanguages(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name)
	if err != nil {
		LogError.Errorf("github list languages failed: %v", err)
	}
	_, ok := languages["Go"]
	if !ok {
		_, ok := languages["Golang"]
		if !ok {
			LogAccess.Info("No need to run go test")
			return nil
		}
	}
	t := github.Timestamp{Time: time.Now()}

	outputTitle := "gotest"
	checkRun, err := CreateCheckRun(ctx, client, outputTitle, gpull, ref, targetURL)
	if err != nil {
		LogError.Errorf("github create check run failed: %v", err)
		return err
	}
	checkRunID := checkRun.GetID()

	outputSummary, err := Gotest(ctx, repo)
	if err != nil {
		return UpdateCheckRun(ctx, client, gpull, checkRunID, "failure", t, outputTitle, outputSummary, nil)
	}
	return UpdateCheckRun(ctx, client, gpull, checkRunID, "success", t, outputTitle, outputSummary, nil)
}
