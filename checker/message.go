package checker

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/tengattack/unified-ci/checks/lint"
	"github.com/tengattack/unified-ci/checks/tester"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/util"
	"golang.org/x/sync/errgroup"
)

// GenerateAnnotations generate github annotations from github diffs and lint option
func GenerateAnnotations(ctx context.Context, ref common.GithubRef, repoPath string, diffs []*diff.FileDiff, lintEnabled lint.LintEnabled,
	ignoredPath []string, log *os.File) (
	outputSummary string, annotations []*github.CheckRunAnnotation, problems int, err error) {
	var (
		annotationsArr [3][]*github.CheckRunAnnotation
		problemsArr    [3]int
		bufArr         [3]strings.Builder
	)

	var eg errgroup.Group
	eg.Go(func() error {
		var err error
		outputSummary, annotationsArr[0], problemsArr[0], err = lint.LintRepo(ctx, ref, repoPath, diffs, lintEnabled, &bufArr[0])
		return err
	})
	eg.Go(func() error {
		var err error
		annotationsArr[1], problemsArr[1], err = lint.LintIndividually(ctx, ref, repoPath, diffs, lintEnabled, ignoredPath, &bufArr[1])
		return err
	})
	eg.Go(func() error {
		var err error
		annotationsArr[2], problemsArr[2], err = lint.LintFileMode(ctx, ref, repoPath, diffs, &bufArr[2])
		return err
	})

	err = eg.Wait()

	annotations = append(annotations, annotationsArr[0]...)
	annotations = append(annotations, annotationsArr[1]...)
	annotations = append(annotations, annotationsArr[2]...)
	problems += problemsArr[0]
	problems += problemsArr[1]
	problems += problemsArr[2]
	log.WriteString(bufArr[0].String())
	log.WriteString(bufArr[1].String())
	log.WriteString(bufArr[2].String())

	return
}

// HandleMessage handles message
func HandleMessage(ctx context.Context, message string) error {
	// 限制总时长为一个小时
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()

	m, err := util.ParseMessage(message)
	if err != nil {
		common.LogAccess.Warnf("parse message %q error: %v", message, err)
		return nil
	}

	if m.CheckType == "tree" {
		// branchs
		common.LogAccess.Infof("Start handling %s", m.String())
	} else {
		// pulls
		common.LogAccess.Infof("Start handling %s", m.String())
	}

	// ref to be checked in the owner/repo
	ref := common.GithubRef{
		Owner:    m.Owner,
		RepoName: m.Repo,

		Sha: m.CommitSHA,
	}
	if m.CheckType == "tree" {
		ref.CheckType = common.CheckTypeBranch
		ref.CheckRef = m.Branch
	} else {
		ref.CheckType = common.CheckTypePRHead
		ref.CheckRef = fmt.Sprintf("pr/%d", m.PRNum)
	}

	targetURL := ""
	if len(common.Conf.Core.CheckLogURI) > 0 {
		targetURL = common.Conf.Core.CheckLogURI + m.Repository() + "/" + ref.Sha + ".log"
	}

	repoLogsPath := filepath.Join(common.Conf.Core.LogsDir, m.Repository())
	_ = os.MkdirAll(repoLogsPath, os.ModePerm)

	log, err := os.Create(filepath.Join(repoLogsPath, fmt.Sprintf("%s.log", ref.Sha)))
	if err != nil {
		return err
	}

	var client *github.Client
	var installationID int64
	var gpull *github.PullRequest

	client, installationID, err = common.GetDefaultAPIClient(ref.Owner)
	if err != nil {
		log.WriteString(err.Error() + "\n")
		log.Close()
		return err
	}

	defer func() {
		if err != nil {
			log.WriteString("Handle message failed: " + err.Error() + "\n")
		} else {
			log.WriteString("done.")
			common.LogAccess.Infof("Finish message: %s", message)
		}
		log.Close()
	}()

	log.WriteString(common.UserAgent() + " Date: " + time.Now().Format(time.RFC1123) + "\n\n")

	if ref.IsBranch() {
		log.WriteString(fmt.Sprintf("Start fetching %s\n", m.String()))
	} else {
		log.WriteString(fmt.Sprintf("Start fetching %s\n", m.String()))

		exist, err := common.SearchGithubPR(ctx, client, m.Repository(), m.CommitSHA)
		if err != nil {
			err = fmt.Errorf("SearchGithubPR error: %v", err)
			return err
		}
		if exist == 0 {
			log.WriteString(fmt.Sprintf("commit: %s no longer exists.\n", m.CommitSHA))
			return nil
		}

		gpull, err = common.GetGithubPull(ctx, client, ref.Owner, ref.RepoName, m.PRNum)
		if err != nil {
			err = fmt.Errorf("GetGithubPull error: %v", err)
			return err
		}
		if gpull.GetState() != "open" {
			log.WriteString("PR " + gpull.GetState() + ".\n")
			return nil
		}
	}

	err = ref.UpdateState(client, common.AppName, "pending", targetURL, "checking")
	if err != nil {
		err = fmt.Errorf("Update pull request status error: %v", err)
		return err
	}

	repoPath := filepath.Join(common.Conf.Core.WorkDir, m.Repository())
	_ = os.MkdirAll(repoPath, os.ModePerm)

	log.WriteString("$ git init\n")
	err = util.RunGitCommand(ref, repoPath, []string{"init"}, nil)
	if err != nil {
		return err
	}

	installationToken, _, err := common.JWTClient.Apps.CreateInstallationToken(ctx, installationID, nil)
	if err != nil {
		return err
	}

	var cloneURL string
	if ref.IsBranch() {
		// branchs
		// TODO: using GetBranch api
		cloneURL = "https://github.com/" + ref.Owner + "/" + ref.RepoName + ".git"
	} else {
		// pulls
		cloneURL = gpull.GetBase().GetRepo().GetCloneURL()
	}
	originURL, err := url.Parse(cloneURL) // e.g. https://github.com/octocat/Hello-World.git
	if err != nil {
		return err
	}
	originURL.User = url.UserPassword("x-access-token", installationToken.GetToken())

	var gitCmds []string
	fetchURL := originURL.String()
	if ref.IsBranch() {
		localBranch := m.Branch

		log.WriteString("$ git fetch -f -u " + cloneURL +
			fmt.Sprintf(" %s:%s\n", m.Branch, localBranch))
		gitCmds = []string{"fetch", "-f", "-u", fetchURL,
			fmt.Sprintf("%s:%s", m.Branch, localBranch)}
	} else {
		localBranch := fmt.Sprintf("pull-%d", m.PRNum)

		// git fetch -f -u https://x-access-token:token@github.com/octocat/Hello-World.git pull/%d/head:pull-%d
		// -u option can be used to bypass the restriction which prevents git from fetching into current branch:
		// link: https://stackoverflow.com/a/32561463/4213218
		log.WriteString("$ git fetch -f -u " + cloneURL +
			fmt.Sprintf(" pull/%d/head:%s\n", m.PRNum, localBranch))
		gitCmds = []string{"fetch", "-f", "-u", fetchURL,
			fmt.Sprintf("pull/%d/head:%s", m.PRNum, localBranch)}
	}
	err = util.RunGitCommand(ref, repoPath, gitCmds, log)
	if err != nil {
		return err
	}

	// git checkout -f <commits>/<branch>
	log.WriteString("$ git checkout -f " + ref.Sha + "\n")
	gitCmds = []string{"checkout", "-f", ref.Sha}
	err = util.RunGitCommand(ref, repoPath, gitCmds, log)
	if err != nil {
		return err
	}

	var diffs []*diff.FileDiff
	if m.CheckType == "pull" {
		// this works not accurately
		// git diff -U3 <base_commits>
		// log.WriteString("$ git diff -U3 " + p.Base.Sha + "\n")
		// cmd = exec.Command("git", "diff", "-U3", p.Base.Sha)
		// cmd.Dir = repoPath
		// out, err := cmd.Output()
		// if err != nil {
		// 	return err
		// }

		// get diff from github
		out, err := common.GetGithubPullDiff(ctx, client, ref.Owner, ref.RepoName, m.PRNum)
		if err != nil {
			err = fmt.Errorf("GetGithubPullDiff error: %v", err)
			return err
		}

		log.WriteString("\nParsing diff...\n\n")
		diffs, err = diff.ParseMultiFileDiff(out)
		if err != nil {
			err = fmt.Errorf("ParseMultiFileDiff error: %v", err)
			return err
		}

		err = common.LabelPRSize(ctx, client, ref, m.PRNum, diffs)
		if err != nil {
			log.WriteString("Label PR error: " + err.Error() + "\n")
			common.LogError.Errorf("Label PR error: %v", err)
			// PASS
		}
	}

	lintEnabled := lint.LintEnabled{}
	lintEnabled.Init(repoPath)

	repoConf, err := util.ReadProjectConfig(repoPath)
	if err != nil {
		err = fmt.Errorf("ReadProjectConfig error: %v", err)
		outputTitle := "wrong ci config"
		log.WriteString(err.Error() + "\n")
		if ref.IsBranch() {
			// Update state to error
			erro := ref.UpdateState(client, common.AppName, "error", targetURL, outputTitle)
			if erro != nil {
				common.LogError.Errorf("Failed to update state to error: %v", erro)
				// PASS
			}
		} else {
			// Can not get tests from config: report action_required instead.
			checkRun, erro := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
			if erro != nil {
				err = fmt.Errorf("Github create check run '%s' failed: %v", outputTitle, erro)
				return err
			}
			UpdateCheckRunWithError(ctx, client, gpull, checkRun.GetID(), outputTitle, outputTitle, err)
		}
		return err
	}

	var (
		failedLints int

		failedTests int
		passedTests int
		errTests    int
		testMsg     string
	)
	noTest := true

	if ref.IsBranch() {
		// only tests
		failedLints = 0
		failedTests, passedTests, errTests, testMsg = checkTests(ctx, repoPath, repoConf.Tests, client, gpull, ref, targetURL, log)
		if failedTests+passedTests+errTests > 0 {
			noTest = false
		}
	} else if repoConf.LinterAfterTests {
		failedTests, passedTests, errTests, testMsg = checkTests(ctx, repoPath, repoConf.Tests, client, gpull, ref, targetURL, log)
		if failedTests+passedTests+errTests > 0 {
			noTest = false
		}

		failedLints, err = checkLints(ctx, client, gpull, ref, targetURL,
			repoPath, diffs, lintEnabled, repoConf.IgnorePatterns, log)
		if err != nil {
			return err
		}
	} else {
		failedLints, err = checkLints(ctx, client, gpull, ref, targetURL,
			repoPath, diffs, lintEnabled, repoConf.IgnorePatterns, log)
		if err != nil {
			return err
		}

		failedTests, passedTests, errTests, testMsg = checkTests(ctx, repoPath, repoConf.Tests, client, gpull, ref, targetURL, log)
		if failedTests+passedTests+errTests > 0 {
			noTest = false
		}
	}
	vulnerabilitiesCount, _ := VulnerabilityCheckRun(ctx, client, gpull, ref, repoPath, targetURL, log)

	mark := '✔'
	sumCount := failedLints + failedTests + vulnerabilitiesCount
	if sumCount > 0 {
		mark = '✖'
	}
	log.WriteString(fmt.Sprintf("%c %d problem(s) found.\n\n",
		mark, sumCount))
	log.WriteString("Updating status...\n")

	var outputSummary string
	// UpdateState: description has a limit of 140 characters
	if sumCount > 0 {
		// update PR state
		outputSummary = fmt.Sprintf("The check failed! %d problem(s) found.\n", sumCount)
		err = ref.UpdateState(client, common.AppName, "error", targetURL, outputSummary)
	} else {
		// update PR state
		outputSummary = "The check succeed!"
		err = ref.UpdateState(client, common.AppName, "success", targetURL, outputSummary)
	}
	if err != nil {
		log.WriteString("UpdateState error: " + err.Error() + "\n")
		common.LogError.Errorf("UpdateState error: %v", err)
		// PASS
	}

	if m.CheckType == "pull" {
		// create review
		if sumCount > 0 {
			comment := fmt.Sprintf("**lint**: %d problem(s) found.\n", failedLints)
			comment += fmt.Sprintf("**vulnerability**: %d problem(s) found.\n", vulnerabilitiesCount)
			if !noTest {
				comment += fmt.Sprintf("**test**: %d problem(s) found.\n\n", failedTests)
				comment += testMsg
			}
			err = ref.CreateReview(client, m.PRNum, "REQUEST_CHANGES", comment, nil)
		} else {
			comment := "**check**: no problems found.\n"
			if !noTest {
				comment += ("\n" + testMsg)
			}
			err = ref.CreateReview(client, m.PRNum, "APPROVE", comment, nil)
		}
		if err != nil {
			err = fmt.Errorf("CreateReview error: %v", err)
		}
	}
	return err
}

// TODO: add test
func checkLints(ctx context.Context, client *github.Client, gpull *github.PullRequest, ref common.GithubRef, targetURL string,
	repoPath string, diffs []*diff.FileDiff, lintEnabled lint.LintEnabled, ignoredPath []string, log *os.File) (problems int, err error) {

	t := github.Timestamp{Time: time.Now()}
	checkName := "linter"
	checkRun, err := CreateCheckRun(ctx, client, gpull, checkName, ref, targetURL)
	if err != nil {
		return 0, err
	}
	checkRunID := checkRun.GetID()

	notes, annotations, failedLints, err := GenerateAnnotations(ctx, ref, repoPath, diffs, lintEnabled, ignoredPath, log)
	if err != nil {
		UpdateCheckRunWithError(ctx, client, gpull, checkRunID, "linter", "linter", err)
		return 0, err
	}

	annotations, filtered := filterLints(ignoredPath, annotations)
	failedLints -= filtered

	if len(annotations) > 50 {
		// TODO: push all
		annotations = annotations[:50]
		common.LogAccess.Warn("Too many annotations to push them all at once. Only 50 annotations will be pushed right now.")
	}

	var (
		conclusion    string
		outputTitle   string
		outputSummary string
	)

	if failedLints > 0 {
		conclusion = "failure"
		outputTitle = fmt.Sprintf("%d problem(s) found.", failedLints)
		outputSummary = fmt.Sprintf("The lint check failed! %d problem(s) found.\n", failedLints)
		if notes != "" {
			outputSummary += "```\n" + notes + "\n```"
		}
	} else {
		conclusion = "success"
		outputTitle = "No problems found."
		outputSummary = "The lint check succeed!"
	}
	err = UpdateCheckRun(ctx, client, gpull, checkRunID, checkName, conclusion, t, outputTitle, outputSummary, annotations)
	return failedLints, err
}

func filterLints(ignoredPath []string, annotations []*github.CheckRunAnnotation) ([]*github.CheckRunAnnotation, int) {
	var filteredAnnotations []*github.CheckRunAnnotation
	for _, a := range annotations {
		if !util.MatchAny(ignoredPath, a.GetPath()) {
			filteredAnnotations = append(filteredAnnotations, a)
		}
	}
	return filteredAnnotations, len(annotations) - len(filteredAnnotations)
}

func checkTests(ctx context.Context, repoPath string, tests map[string]util.TestsConfig,
	client *github.Client, gpull *github.PullRequest, ref common.GithubRef,
	targetURL string, log *os.File) (failedTests, passedTests, errTests int, testMsg string) {

	var baseSHA string
	if !ref.IsBranch() {
		// compare test coverage with base
		var err error
		baseSHA, err = util.GetBaseSHA(ctx, client, ref.Owner, ref.RepoName, gpull.GetNumber())
		if err != nil {
			msg := fmt.Sprintf("Cannot get BaseSHA: %v\n", err)
			common.LogError.Error(msg)
			log.WriteString(msg)
			return
		}
		ref.BaseSha = baseSHA
	}
	var headCoverage sync.Map
	failedTests, passedTests, errTests, _ = TestCheckRun(ctx,
		repoPath, client, gpull, ref,
		targetURL, tests, &headCoverage, log)

	if !ref.IsBranch() {
		baseSavedRecords, baseTestsNeedToRun := tester.LoadBaseFromStore(ref, baseSHA, tests, log)
		var baseCoverage sync.Map
		_ = tester.FindBaseCoverage(ctx, baseSavedRecords, baseTestsNeedToRun, repoPath, baseSHA, gpull, ref, log, &baseCoverage)
		testMsg = util.DiffCoverage(&headCoverage, &baseCoverage)
	}
	return
}
