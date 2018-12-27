package checker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
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

func pickDiffLintMessages(lintsDiff []LintMessage, d *diff.FileDiff, annotations []*github.CheckRunAnnotation, problems int, log *os.File, fileName string) ([]*github.CheckRunAnnotation, int) {
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
				annotations = append(annotations, &github.CheckRunAnnotation{
					Path:            &fileName,
					Message:         &comment,
					StartLine:       &startLine,
					EndLine:         &startLine,
					AnnotationLevel: &annotationLevel,
				})
				problems++
				break
			}
		}
	}
	return annotations, problems
}

// GenerateComments generate github comments from github diffs and lint option
func GenerateComments(repoPath string, diffs []*diff.FileDiff, lintEnabled *LintEnabled, log *os.File) ([]*github.CheckRunAnnotation, int, error) {
	annotations := []*github.CheckRunAnnotation{}
	annotationLevel := "warning" // TODO: from lint.Severity
	problems := 0
	var lintErr error
	for _, d := range diffs {
		newName, err := strconv.Unquote(d.NewName)
		if err != nil {
			newName = d.NewName
		}
		if strings.HasPrefix(newName, "b/") {
			fileName := newName[2:]
			log.WriteString(fmt.Sprintf("Checking '%s'\n", fileName))
			var lints []LintMessage
			if lintEnabled.MD && strings.HasSuffix(fileName, ".md") {
				log.WriteString(fmt.Sprintf("Markdown '%s'\n", fileName))
				rps, out, err := remark(fileName, repoPath)
				if err != nil {
					return nil, 0, err
				}
				lintsFormatted, err := MDFormattedLint(filepath.Join(repoPath, fileName), out)
				if err != nil {
					return nil, 0, err
				}
				annotations, problems = pickDiffLintMessages(lintsFormatted, d, annotations, problems, log, fileName)
				lints, lintErr = MDLint(rps)
			} else if lintEnabled.CPP && isCPP(fileName) {
				log.WriteString(fmt.Sprintf("CPPLint '%s'\n", fileName))
				lints, lintErr = CPPLint(fileName, repoPath)
			} else if lintEnabled.Go && strings.HasSuffix(fileName, ".go") {
				log.WriteString(fmt.Sprintf("Goreturns '%s'\n", fileName))
				lintsGoreturns, err := Goreturns(filepath.Join(repoPath, fileName), repoPath)
				if err != nil {
					return nil, 0, err
				}
				annotations, problems = pickDiffLintMessages(lintsGoreturns, d, annotations, problems, log, fileName)
				log.WriteString(fmt.Sprintf("Golint '%s'\n", fileName))
				lints, lintErr = Golint(filepath.Join(repoPath, fileName), repoPath)
			} else if lintEnabled.PHP && strings.HasSuffix(fileName, ".php") {
				log.WriteString(fmt.Sprintf("PHPLint '%s'\n", fileName))
				lints, lintErr = PHPLint(filepath.Join(repoPath, fileName), repoPath)
			} else if lintEnabled.TypeScript && (strings.HasSuffix(fileName, ".ts") ||
				strings.HasSuffix(fileName, ".tsx")) {
				log.WriteString(fmt.Sprintf("TSLint '%s'\n", fileName))
				lints, lintErr = TSLint(filepath.Join(repoPath, fileName), repoPath)
			} else if lintEnabled.SCSS && (strings.HasSuffix(fileName, ".scss") ||
				strings.HasSuffix(fileName, ".css")) {
				log.WriteString(fmt.Sprintf("SCSSLint '%s'\n", fileName))
				lints, lintErr = SCSSLint(filepath.Join(repoPath, fileName), repoPath)
			} else if lintEnabled.JS != "" && strings.HasSuffix(fileName, ".js") {
				log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
				lints, lintErr = ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
			} else if lintEnabled.ES != "" && (strings.HasSuffix(fileName, ".es") ||
				strings.HasSuffix(fileName, ".esx") || strings.HasSuffix(fileName, ".jsx")) {
				log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
				lints, lintErr = ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.ES)
			}
			if lintErr != nil {
				return nil, 0, lintErr
			}
			if lintEnabled.JS != "" && (strings.HasSuffix(fileName, ".html") ||
				strings.HasSuffix(fileName, ".php")) {
				// ESLint for HTML & PHP files (ES5)
				log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
				lints2, err := ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
				if err != nil {
					return nil, 0, err
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
									annotations = append(annotations, &github.CheckRunAnnotation{
										Path:            &fileName,
										Message:         &comment,
										StartLine:       &startLine,
										EndLine:         &startLine,
										AnnotationLevel: &annotationLevel,
									})
									// ref.CreateComment(repository, pull, fileName,
									// 	int(hunk.StartPosition)+i, comment)
									problems++
								}
							}
						}
					}
				} // end for
			}
			log.WriteString("\n")
		}
	}
	return annotations, problems, nil
}

// HandleMessage handles message
func HandleMessage(message string) error {
	s := strings.Split(message, "/")
	if len(s) != 6 || s[2] != "pull" || s[4] != "commits" {
		LogAccess.Warnf("malfromed message: %s", message)
		return nil
	}

	owner := s[0]
	repository, pull, commitSha := s[0]+"/"+s[1], s[3], s[5]
	LogAccess.Infof("Start fetching %s/pull/%s", repository, pull)

	ref := GithubRef{
		RepoName: repository,
		Sha:      commitSha,
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
	var gpull *GithubPull

	// Wrap the shared transport for use with the integration ID authenticating with installation ID.
	// TODO: add installation ID to db
	installationID, ok := Conf.GitHub.Installations[owner]
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

	log.WriteString("Pull Request Checker/" + GetVersion() + "\n\n")
	log.WriteString(fmt.Sprintf("Start fetching %s/pull/%s\n", repository, pull))

	gpull, err = GetGithubPull(repository, pull)
	if err != nil {
		return err
	}
	if gpull.State != "open" {
		log.WriteString("PR " + gpull.State + ".")
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

	err = ref.UpdateState("lint", "pending", targetURL, "checking")
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

	log.WriteString("$ git remote add " + gpull.Head.User.Login + " " + gpull.Head.Repo.SSHURL + "\n")
	cmd = exec.Command("git", "remote", "add", gpull.Head.User.Login, gpull.Head.Repo.SSHURL)
	cmd.Dir = repoPath
	err = cmd.Run()
	if err != nil {
		LogAccess.Debugf("git remote add %s", err.Error())
		// return err
	}

	// git fetch -f origin pull/XX/head:pull-XX
	branch := fmt.Sprintf("pull-%s", pull)
	log.WriteString("$ git fetch -f " + gpull.Head.User.Login + " " +
		fmt.Sprintf("%s:%s\n", gpull.Head.Ref, branch))
	cmd = exec.Command("git", "fetch", "-f", gpull.Head.User.Login,
		fmt.Sprintf("%s:%s", gpull.Head.Ref, branch))
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
	out, err := GetGithubPullDiff(repository, pull)
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

	var futures []chan error
	tests := getTests(diffs)
	for k := range tests {
		switch k {
		case "go":
			if Conf.Core.Gotest != "" {
				future := ReportTestResults(repoPath, Conf.Core.Gotest, "./...", client, gpull, "gotest", ref, targetURL)
				futures = append(futures, future)
			}
		case "php":
			if Conf.Core.PHPUnit != "" {
				future := ReportTestResults(repoPath, Conf.Core.PHPUnit, "", client, gpull, "phptest", ref, targetURL)
				futures = append(futures, future)
			}
		}
	}

	annotations, problems, err := GenerateComments(repoPath, diffs, &lintEnabled, log)
	if err != nil {
		return err
	}

	for _, v := range futures {
		errReport, exist := <-v
		if exist {
			if _, ok := errReport.(*testResultProblemFound); ok {
				problems++
			} else {
				log.WriteString(fmt.Sprintf("Trouble in ReportTestResults: %v\n", errReport))
			}
		}
	}
	mark := '✔'
	if problems > 0 {
		mark = '✖'
	}
	log.WriteString(fmt.Sprintf("%c %d problem(s) found.\n\n",
		mark, problems))
	log.WriteString("Updating status...\n")

	var conclusion string
	var outputSummary string
	if problems > 0 {
		comment := fmt.Sprintf("**lint**: %d problem(s) found.", problems)
		err = ref.CreateReview(pull, "REQUEST_CHANGES", comment, nil)
		if err != nil {
			log.WriteString("error: " + err.Error() + "\n")
			LogError.Errorf("create review failed: %v", err)
		}
		conclusion = "failure"
		outputSummary = fmt.Sprintf("The lint check failed! %d problem(s) found.", problems)
		err = ref.UpdateState("lint", "error", targetURL, outputSummary)
	} else {
		err = ref.CreateReview(pull, "APPROVE", "**lint**: no problems found.", nil)
		if err != nil {
			log.WriteString("error: " + err.Error() + "\n")
			LogError.Errorf("create review failed: %v", err)
		}
		conclusion = "success"
		outputSummary = "The lint check succeed!"
		err = ref.UpdateState("lint", "success", targetURL, outputSummary)
	}
	if err == nil {
		log.WriteString("done.")
	}

	if checkRunID != 0 {
		if len(annotations) > 50 {
			// TODO: push all
			annotations = annotations[:50]
			LogAccess.Warn("Too many annotations to push them all at once. Only 50 annotations will be pushed right now.")
		}
		err = UpdateCheckRun(ctx, client, gpull, checkRunID, outputTitle, conclusion, t, outputTitle, outputSummary, annotations)
	}
	return err
}
