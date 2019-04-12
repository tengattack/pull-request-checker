package checker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
	"golang.org/x/sync/errgroup"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

func isCPP(fileName string) bool {
	ext := []string{".c", ".cc", ".h", ".hpp", ".c++", ".h++", ".cu", ".cpp", ".hxx", ".cxx", ".cuh"}
	for i := 0; i < len(ext); i++ {
		if strings.HasSuffix(fileName, ext[i]) {
			return true
		}
	}
	return false
}

func pickDiffLintMessages(lintsDiff []LintMessage, d *diff.FileDiff, annotations *[]*github.CheckRunAnnotation, problems *int, log *bytes.Buffer, fileName string) {
	annotationLevel := "warning" // TODO: from lint.Severity
	for _, lint := range lintsDiff {
		for _, hunk := range d.Hunks {
			intersection := lint.Column > 0 && hunk.NewLines > 0
			intersection = intersection && (lint.Line+lint.Column-1 >= int(hunk.NewStartLine))
			intersection = intersection && (int(hunk.NewStartLine+hunk.NewLines-1) >= lint.Line)
			if intersection {
				log.WriteString(fmt.Sprintf("%d:%d %s %s\n",
					lint.Line, 0, lint.Message, lint.RuleID))
				comment := fmt.Sprintf("`%s` %d:%d %s",
					lint.RuleID, lint.Line, 0, lint.Message)
				startLine := lint.Line
				endline := startLine + lint.Column - 1
				*annotations = append(*annotations, &github.CheckRunAnnotation{
					Path:            &fileName,
					Message:         &comment,
					StartLine:       &startLine,
					EndLine:         &endline,
					AnnotationLevel: &annotationLevel,
				})
				*problems++
				break
			}
		}
	}
}

// GenerateAnnotations generate github annotations from github diffs and lint option
func GenerateAnnotations(repoPath string, diffs []*diff.FileDiff, lintEnabled LintEnabled, log *os.File) ([]*github.CheckRunAnnotation, int, error) {
	annotationLevel := "warning" // TODO: from lint.Severity
	maxPending := Conf.Concurrency.Lint
	if maxPending < 1 {
		maxPending = 1
	}
	pending := make(chan int, maxPending)
	var (
		eg  errgroup.Group
		mtx sync.Mutex

		annotations []*github.CheckRunAnnotation
		problems    int
	)
	for _, d := range diffs {
		pending <- 0
		d := d
		eg.Go(func() error {
			defer func() { <-pending }()
			var (
				buf          bytes.Buffer
				annotations_ []*github.CheckRunAnnotation
				problems_    int
			)

			err := handleSingleFile(repoPath, d, lintEnabled, annotationLevel, &buf, &annotations_, &problems_)

			mtx.Lock()
			defer mtx.Unlock()
			log.Write(buf.Bytes())
			annotations = append(annotations, annotations_...)
			problems += problems_

			return err
		})
	}
	err := eg.Wait()
	// The check-run status will be set to "action_required" if err != nil
	return annotations, problems, err
}

func handleSingleFile(repoPath string, d *diff.FileDiff, lintEnabled LintEnabled, annotationLevel string, log *bytes.Buffer, annotations *[]*github.CheckRunAnnotation, problems *int) error {
	newName, err := strconv.Unquote(d.NewName)
	if err != nil {
		newName = d.NewName
	}
	if !strings.HasPrefix(newName, "b/") {
		log.WriteString("No need to process " + newName)
		return nil
	}
	fileName := newName[2:]
	log.WriteString(fmt.Sprintf("Checking '%s'\n", fileName))

	var lints []LintMessage
	var lintErr error
	if lintEnabled.MD && strings.HasSuffix(fileName, ".md") {
		log.WriteString(fmt.Sprintf("Markdown '%s'\n", fileName))
		rps, out, err := remark(fileName, repoPath)
		if err != nil {
			return err
		}
		lintsFormatted, err := MDFormattedLint(filepath.Join(repoPath, fileName), out)
		if err != nil {
			return err
		}
		pickDiffLintMessages(lintsFormatted, d, annotations, problems, log, fileName)
		lints, lintErr = MDLint(rps)
	} else if lintEnabled.CPP && isCPP(fileName) {
		log.WriteString(fmt.Sprintf("CPPLint '%s'\n", fileName))
		lints, lintErr = CPPLint(fileName, repoPath)
	} else if lintEnabled.Go && strings.HasSuffix(fileName, ".go") {
		log.WriteString(fmt.Sprintf("Goreturns '%s'\n", fileName))
		lintsGoreturns, err := Goreturns(filepath.Join(repoPath, fileName), repoPath)
		if err != nil {
			return err
		}
		pickDiffLintMessages(lintsGoreturns, d, annotations, problems, log, fileName)
		log.WriteString(fmt.Sprintf("Golint '%s'\n", fileName))
		lints, lintErr = Golint(filepath.Join(repoPath, fileName), repoPath)
	} else if lintEnabled.PHP && strings.HasSuffix(fileName, ".php") {
		log.WriteString(fmt.Sprintf("PHPLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = PHPLint(filepath.Join(repoPath, fileName), repoPath)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	} else if lintEnabled.TypeScript && (strings.HasSuffix(fileName, ".ts") ||
		strings.HasSuffix(fileName, ".tsx")) {
		log.WriteString(fmt.Sprintf("TSLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = TSLint(filepath.Join(repoPath, fileName), repoPath)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	} else if lintEnabled.SCSS && (strings.HasSuffix(fileName, ".scss") ||
		strings.HasSuffix(fileName, ".css")) {
		log.WriteString(fmt.Sprintf("SCSSLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = SCSSLint(filepath.Join(repoPath, fileName), repoPath)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	} else if lintEnabled.JS != "" && strings.HasSuffix(fileName, ".js") {
		log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	} else if lintEnabled.ES != "" && (strings.HasSuffix(fileName, ".es") ||
		strings.HasSuffix(fileName, ".esx") || strings.HasSuffix(fileName, ".jsx")) {
		log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.ES)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	}
	if lintErr != nil {
		return lintErr
	}
	if lintEnabled.JS != "" && (strings.HasSuffix(fileName, ".html") ||
		strings.HasSuffix(fileName, ".php")) {
		// ESLint for HTML & PHP files (ES5)
		log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
		lints2, errlog, err := ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
		if err != nil {
			return err
		}
		if lints2 != nil {
			if lints != nil {
				lints = append(lints, lints2...)
			} else {
				lints = lints2
			}
		}
	}

	if lints != nil {
		for _, hunk := range d.Hunks {
			if hunk.NewLines > 0 {
				lines := strings.Split(string(hunk.Body), "\n")
				for _, lint := range lints {
					if lint.Line >= int(hunk.NewStartLine) &&
						lint.Line < int(hunk.NewStartLine+hunk.NewLines) {
						lineNum := 0
						i := 0
						lastLineFromOrig := true
						for ; i < len(lines); i++ {
							lineExists := len(lines[i]) > 0
							if !lineExists || lines[i][0] != '-' {
								if lineExists && lines[i][0] == '\\' && lastLineFromOrig {
									// `\ No newline at end of file` from original source file
									continue
								}
								if lineNum <= 0 {
									lineNum = int(hunk.NewStartLine)
								} else {
									lineNum++
								}
							}
							if lineNum >= lint.Line {
								break
							}
							if lineExists {
								lastLineFromOrig = lines[i][0] == '-'
							}
						}
						if i < len(lines) && len(lines[i]) > 0 && lines[i][0] == '+' {
							// ensure this line is a definitely new line
							log.WriteString(lines[i] + "\n")
							log.WriteString(fmt.Sprintf("%d:%d %s %s\n",
								lint.Line, lint.Column, lint.Message, lint.RuleID))
							comment := fmt.Sprintf("`%s` %d:%d %s",
								lint.RuleID, lint.Line, lint.Column, lint.Message)
							startLine := lint.Line
							*annotations = append(*annotations, &github.CheckRunAnnotation{
								Path:            &fileName,
								Message:         &comment,
								StartLine:       &startLine,
								EndLine:         &startLine,
								AnnotationLevel: &annotationLevel,
							})
							// ref.CreateComment(repository, pull, fileName,
							// 	int(hunk.StartPosition)+i, comment)
							*problems++
						}
					}
				}
			}
		} // end for
	}
	log.WriteString("\n")
	return nil
}

// HandleMessage handles message
func HandleMessage(message string) error {
	s := strings.Split(message, "/")
	if len(s) != 6 || s[2] != "pull" || s[4] != "commits" {
		LogAccess.Warnf("malformed message: %s", message)
		return nil
	}

	repository, pull, commitSha := s[0]+"/"+s[1], s[3], s[5]
	prNum, err := strconv.Atoi(pull)
	if err != nil {
		LogAccess.Warnf("malformed message: %s", message)
		return err
	}
	LogAccess.Infof("Start handling %s/pull/%s", repository, pull)

	ref := GithubRef{
		owner: s[0],
		repo:  s[1],
		Sha:   commitSha,
	}
	targetURL := ""
	if len(Conf.Core.CheckLogURI) > 0 {
		targetURL = Conf.Core.CheckLogURI + repository + "/" + ref.Sha + ".log"
	}

	repoLogsPath := filepath.Join(Conf.Core.LogsDir, repository)
	os.MkdirAll(repoLogsPath, os.ModePerm)

	log, err := os.Create(filepath.Join(repoLogsPath, fmt.Sprintf("%s.log", ref.Sha)))
	if err != nil {
		return err
	}

	var checkRunID int64
	var client *github.Client
	var gpull *github.PullRequest

	// Wrap the shared transport for use with the integration ID authenticating with installation ID.
	// TODO: add installation ID to db
	installationID, ok := Conf.GitHub.Installations[ref.owner]
	if ok {
		tr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport,
			Conf.GitHub.AppID, installationID, Conf.GitHub.PrivateKey)
		if err != nil {
			LogError.Errorf("load private key failed: %v", err)
			// close log manually
			log.WriteString("error: " + err.Error() + "\n")
			log.Close()
			return err
		}

		// TODO: refine code
		client = github.NewClient(&http.Client{Transport: tr})
	} else {
		msg := "Installation ID not found, owner: " + ref.owner
		LogError.Error(msg)
		// close log manually
		log.WriteString(msg + "\n")
		log.Close()
		return errors.New(msg)
	}

	defer func() {
		if err != nil {
			LogError.Errorf("handle message failed: %v", err)
			log.WriteString("error: " + err.Error() + "\n")
		} else {
			LogAccess.Infof("Finish message: %s", message)
		}
		if err != nil && checkRunID != 0 {
			ctx := context.Background()
			UpdateCheckRunWithError(ctx, client, gpull, checkRunID, "linter", "linter", err)
		}
		log.Close()
	}()

	exist, err := searchGithubPR(context.Background(), client, repository, commitSha)
	if err != nil {
		LogAccess.Errorf("searchGithubPR error: %v", err)
		return err
	}
	if exist == 0 {
		log.WriteString(fmt.Sprintf("commit:%s no longer exists.\n", commitSha))
		return nil
	}

	log.WriteString(UserAgent() + "\n\n")
	log.WriteString(fmt.Sprintf("Start fetching %s/pull/%s\n", repository, pull))

	gpull, err = GetGithubPull(client, ref.owner, ref.repo, prNum)
	if err != nil {
		return err
	}
	if gpull.GetState() != "open" {
		log.WriteString("PR " + gpull.GetState() + ".")
		return nil
	}

	t := github.Timestamp{Time: time.Now()}
	ctx := context.Background()
	outputTitle := "linter"
	checkRun, err := CreateCheckRun(ctx, client, gpull, outputTitle, ref, targetURL)
	if err != nil {
		LogError.Errorf("github create check run failed: %v", err)
		// PASS
	} else {
		checkRunID = checkRun.GetID()
	}

	err = ref.UpdateState(client, AppName, "pending", targetURL, "checking")
	if err != nil {
		LogAccess.Error("Update pull request status error: " + err.Error())
		return err
	}

	repoPath := filepath.Join(Conf.Core.WorkDir, repository)
	os.MkdirAll(repoPath, os.ModePerm)

	log.WriteString("$ git init\n")
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	err = cmd.Run()
	if err != nil {
		return err
	}

	installationToken, _, err := jwtClient.Apps.CreateInstallationToken(context.Background(), int64(installationID))
	if err != nil {
		return err
	}

	originURL, err := url.Parse(gpull.GetBase().GetRepo().GetCloneURL()) // e.g. https://github.com/octocat/Hello-World.git
	if err != nil {
		return err
	}
	originURL.User = url.UserPassword("x-access-token", installationToken.GetToken())
	fetchURL := originURL.String()

	// git fetch -f https://x-access-token:token@github.com/octocat/Hello-World.git new-topic:pull-XX
	branch := fmt.Sprintf("pull-%d", prNum)
	// -u option can be used to bypass the restriction which prevents git from fetching into current branch:
	// link: https://stackoverflow.com/a/32561463/4213218
	log.WriteString("$ git fetch -f -u " + gpull.GetBase().GetRepo().GetCloneURL() + fmt.Sprintf(" pull/%d/head:%s\n", prNum, branch))
	cmd = exec.Command("git", "fetch", "-f", "-u", fetchURL, fmt.Sprintf("pull/%d/head:%s", prNum, branch))
	cmd.Dir = repoPath
	cmd.Stdout = log
	cmd.Stderr = log
	err = cmd.Run()
	if err != nil {
		return err
	}

	// git checkout -f <commits>/<branch>
	log.WriteString("$ git checkout -f " + ref.Sha + "\n")
	cmd = exec.Command("git", "checkout", "-f", ref.Sha)
	cmd.Dir = repoPath
	cmd.Stdout = log
	cmd.Stderr = log
	err = cmd.Run()
	if err != nil {
		return err
	}

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
	out, err := GetGithubPullDiff(client, ref.owner, ref.repo, prNum)
	if err != nil {
		return err
	}

	log.WriteString("\nParsing diff...\n\n")
	diffs, err := diff.ParseMultiFileDiff(out)
	if err != nil {
		return err
	}

	lintEnabled := LintEnabled{}
	lintEnabled.Init(repoPath)
	annotations, failedLints, err := GenerateAnnotations(repoPath, diffs, lintEnabled, log)
	if err != nil {
		return err
	}

	noTest := true
	failedTests, passedTests, errTests := runTest(repoPath, client, gpull, ref, targetURL, log)
	if failedTests+passedTests+errTests > 0 {
		noTest = false
	}

	mark := '✔'
	sumCount := failedLints + failedTests
	if sumCount > 0 {
		mark = '✖'
	}
	log.WriteString(fmt.Sprintf("%c %d problem(s) found.\n\n",
		mark, sumCount))
	log.WriteString("Updating status...\n")

	var outputSummary string
	if sumCount > 0 {
		comment := fmt.Sprintf("**lint**: %d problem(s) found.\n", failedLints)
		if !noTest {
			comment += fmt.Sprintf("**test**: %d problem(s) found.\n", failedTests)
		}

		err = ref.CreateReview(client, prNum, "REQUEST_CHANGES", comment, nil)
		if err != nil {
			log.WriteString("error: " + err.Error() + "\n")
			LogError.Errorf("create review failed: %v", err)
		}
		outputSummary = fmt.Sprintf("The check failed! %d problem(s) found.", sumCount)
		err = ref.UpdateState(client, AppName, "error", targetURL, outputSummary)
	} else {
		err = ref.CreateReview(client, prNum, "APPROVE", "**check**: no problems found.", nil)
		if err != nil {
			log.WriteString("error: " + err.Error() + "\n")
			LogError.Errorf("create review failed: %v", err)
		}
		outputSummary = "The check succeed!"
		err = ref.UpdateState(client, AppName, "success", targetURL, outputSummary)
	}
	if err == nil {
		log.WriteString("done.")
	} else {
		log.WriteString("Failed to update status: " + err.Error())
	}

	if checkRunID != 0 {
		if len(annotations) > 50 {
			// TODO: push all
			annotations = annotations[:50]
			LogAccess.Warn("Too many annotations to push them all at once. Only 50 annotations will be pushed right now.")
		}
		var conclusion string
		if failedLints > 0 {
			conclusion = "failure"
			outputSummary = fmt.Sprintf("The lint check failed! %d problem(s) found.", failedLints)
		} else {
			conclusion = "success"
			outputSummary = "The lint check succeed!"
		}
		err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, outputTitle, outputSummary, annotations)
	}
	return err
}

func runTest(repoPath string, client *github.Client, gpull *github.PullRequest, ref GithubRef, targetURL string, log *os.File) (failedTests, passedTests, errTests int) {
	maxPendingTests := Conf.Concurrency.Test
	if maxPendingTests < 1 {
		maxPendingTests = 1
	}
	pendingTests := make(chan int, maxPendingTests)
	errReports := make(chan error, maxPendingTests)
	tests, err := getTests(repoPath)
	if err != nil {
		// Can not get tests from config: report action_required instead.
		outputTitle := "tests"
		checkRun, err := CreateCheckRun(context.TODO(), client, gpull, outputTitle, ref, targetURL)
		if err != nil {
			msg := fmt.Sprintf("github create check run '%s' failed: %v\n", outputTitle, err)
			LogError.Error(msg)
			log.WriteString(msg)
			return
		}
		UpdateCheckRunWithError(context.TODO(), client, gpull, checkRun.GetID(), outputTitle, outputTitle, err)
		return
	}

	done := make(chan struct{})
	go func() {
		// This goroutine is the only reader of the errReports channel.
		// It is ready to quit when the errReports channel is closed and drained.
		for errReport := range errReports {
			if errReport != nil {
				if _, ok := errReport.(*testNotPass); ok {
					failedTests++
				} else {
					errTests++
				}
			} else {
				passedTests++
			}
		}
		close(done)
	}()

	var wg sync.WaitGroup
	for k, v := range tests {
		testName := k
		cmds := v

		pendingTests <- 0
		wg.Add(1)
		go func() {
			// This goroutine is a writer of the errReports channel
			defer func() {
				if info := recover(); info != nil {
					errReports <- &panicError{Info: info}
				}
				wg.Done()
				<-pendingTests
			}()
			errReports <- ReportTestResults(repoPath, cmds, client, gpull, testName+" test", ref, targetURL)
		}()
	}
	// We must wait for all writers to quit before we close the errReports channel. Otherwise it will panic.
	wg.Wait()
	// Tell the reader that it may quit
	close(errReports)
	// Wait for the reader to quit
	<-done
	return
}
