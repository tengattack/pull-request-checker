package util

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/go-github/github"
)

// SearchGithubPR searches for the PR number of one commit
func SearchGithubPR(ctx context.Context, client *github.Client, repo, sha string) (int, error) {
	if sha == "" {
		return 0, errors.New("SHA is empty")
	}
	q := fmt.Sprintf("is:pr repo:%s SHA:%s", repo, sha)
	opts := &github.SearchOptions{Sort: "created", Order: "asc"}
	result, _, err := client.Search.Issues(ctx, q, opts)
	if err != nil {
		return 0, err
	}
	if len(result.Issues) == 0 {
		return 0, nil
	}
	return result.Issues[0].GetNumber(), nil
}

// DiffCoverage generates a diff-format message to show the test coverage's difference between head and base.
func DiffCoverage(headCoverage, baseCoverage *sync.Map) string {
	var output strings.Builder
	headCoverage.Range(func(key, value interface{}) bool {
		testName, _ := key.(string)
		currentResult, _ := value.(string)

		interfaceValue, _ := baseCoverage.Load(testName)
		baseResult, _ := interfaceValue.(string)

		currentPercentage, _, err1 := ParseFloatPercent(currentResult, 64)
		basePercentage, _, err2 := ParseFloatPercent(baseResult, 64)
		if err1 != nil || err2 != nil {
			output.WriteString("  ")
		} else if currentPercentage > basePercentage {
			output.WriteString("+ ")
		} else if currentPercentage < basePercentage {
			output.WriteString("- ")
		} else {
			output.WriteString("  ")
		}
		output.WriteString(testName)
		output.WriteString(" test coverage: ")
		output.WriteString(baseResult)
		output.WriteString(" -> ")
		output.WriteString(currentResult)
		output.WriteRune('\n')
		return true
	})
	if output.Len() > 0 {
		output.WriteString("\n```")
		return "```diff\n" + output.String()
	}
	return ""
}

// GetBaseSHA gets the SHA string of the commit which the pull request is based on
func GetBaseSHA(ctx context.Context, client *github.Client, owner, repo string, prNum int) (string, error) {
	opt := &github.ListOptions{Page: 1, PerPage: 1}
	commits, _, err := client.PullRequests.ListCommits(ctx, owner, repo, prNum, opt)
	if err != nil {
		return "", err
	}
	var baseSHA string
	if len(commits) > 0 {
		if length := len(commits[0].Parents); length > 0 {
			baseSHA = commits[0].Parents[length-1].GetSHA()
		}
	}
	return baseSHA, nil
}
