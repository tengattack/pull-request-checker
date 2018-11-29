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

func pickDiffLintMessages(lintsDiff []LintMessage, d *diff.FileDiff, comments []GithubRefComment, problems int, log *os.File, fileName string) ([]GithubRefComment, int) {
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
				comments = append(comments, GithubRefComment{
					Path:     fileName,
					Position: int(hunk.StartPosition) + getOffsetToUnifiedDiff(lint.Line, hunk),
					Body:     comment,
				})
				problems++
				break
			}
		}
	}
	return comments, problems
}

// GenerateComments generate github comments from github diffs and lint option
func GenerateComments(repoPath string, diffs []*diff.FileDiff, lintEnabled *LintEnabled, log *os.File) ([]GithubRefComment, []*github.CheckRunAnnotation, int, error) {
	comments := []GithubRefComment{}
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
					return nil, nil, 0, err
				}
				lintsFormatted, err := MDFormattedLint(filepath.Join(repoPath, fileName), out)
				if err != nil {
					return nil, nil, 0, err
				}
				comments, problems = pickDiffLintMessages(lintsFormatted, d, comments, problems, log, fileName)
				lints, lintErr = MDLint(rps)
			} else if lintEnabled.CPP && isCPP(fileName) {
				log.WriteString(fmt.Sprintf("CPPLint '%s'\n", fileName))
				lints, lintErr = CPPLint(fileName, repoPath)
			} else if lintEnabled.Go && strings.HasSuffix(fileName, ".go") {
				log.WriteString(fmt.Sprintf("Goreturns '%s'\n", fileName))
				lintsGoreturns, err := Goreturns(filepath.Join(repoPath, fileName), repoPath)
				if err != nil {
					return nil, nil, 0, err
				}
				comments, problems = pickDiffLintMessages(lintsGoreturns, d, comments, problems, log, fileName)
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
				return nil, nil, 0, lintErr
			}
			if lintEnabled.JS != "" && (strings.HasSuffix(fileName, ".html") ||
				strings.HasSuffix(fileName, ".php")) {
				// ESLint for HTML & PHP files (ES5)
				log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
				lints2, err := ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
				if err != nil {
					return nil, nil, 0, err
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
									annotations = append(annotations, &github.CheckRunAnnotation{
										Path:            &fileName,
										Message:         &comment,
										StartLine:       &lint.Line,
										EndLine:         &lint.Line,
										AnnotationLevel: &annotationLevel,
									})
									comments = append(comments, GithubRefComment{
										Path:     fileName,
										Position: int(hunk.StartPosition) + i,
										Body:     comment,
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
	return comments, annotations, problems, nil
}

// HandleMessage handles message
func HandleMessage(message string) error {
	s := strings.Split(message, "/")
	if len(s) != 6 || s[2] != "pull" || s[4] != "commits" {
		LogAccess.Warnf("malfromed message: %s", message)
		return nil
	}

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
	var gpull *GithubPull

	// Wrap the shared transport for use with the integration ID authenticating with installation ID.
	// TODO: add installation ID to db
	tr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport,
		Conf.GitHub.AppID, 479595, Conf.GitHub.PrivateKey)
	if err != nil {
		LogError.Errorf("load private key failed: %v", err)
		// close log manually
		log.WriteString("error: " + err.Error() + "\n")
		log.Close()
		return err
	}

	// TODO: refine code
	ctx := context.Background()
	client := github.NewClient(&http.Client{Transport: tr})

	defer func() {
		if err != nil {
			LogError.Errorf("handle message failed: %v", err)
			log.WriteString("error: " + err.Error() + "\n")
		} else {
			LogAccess.Infof("Finish message: %s", message)
		}
		if err != nil && checkRunID != 0 && gpull != nil {
			// update check run result with error message
			conclusion := "action_required"
			checkRunStatus := "completed"
			t := github.Timestamp{Time: time.Now()}
			outputTitle := "linter"
			outputSummary := fmt.Sprintf("error: %v", err)
			_, _, err := client.Checks.UpdateCheckRun(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name, checkRunID, github.UpdateCheckRunOptions{
				Name:        "linter",
				Status:      &checkRunStatus,
				Conclusion:  &conclusion,
				CompletedAt: &t,
				Output: &github.CheckRunOutput{
					Title:   &outputTitle,
					Summary: &outputSummary,
				},
			})
			if err != nil {
				LogError.Errorf("github update check run failed: %v", err)
			}
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
	outputTitle := "linter"
	checkRunStatus := "in_progress"
	checkRun, _, err := client.Checks.CreateCheckRun(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name, github.CreateCheckRunOptions{
		Name:       "linter",
		HeadBranch: gpull.Base.Ref,
		HeadSHA:    ref.Sha,
		DetailsURL: &targetURL,
		Status:     &checkRunStatus,
	})
	if err != nil {
		LogError.Errorf("github create check run failed: %v", err)
		return err
	}
	checkRunID = checkRun.GetID()

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

	comments, annotations, problems, err := GenerateComments(repoPath, diffs, &lintEnabled, log)
	if err != nil {
		return err
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
		// The API doc didn't quite say this but too many comments will cause CreateReview to fail
		// with "HTTP 422 Unprocessable Entity: submitted too quickly"
		// TODO: remove comments for review
		if len(comments) > 30 {
			comments = comments[:30]
			LogAccess.Warn("Too many comments to push them all at once. Only 30 comments will be pushed right now.")
		}
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

	checkRunStatus = "completed"
	if len(annotations) > 50 {
		// TODO: push all
		annotations = annotations[:50]
		LogAccess.Warn("Too many annotations to push them all at once. Only 50 annotations will be pushed right now.")
	}
	_, _, err = client.Checks.UpdateCheckRun(ctx, gpull.Base.Repo.Owner.Login, gpull.Base.Repo.Name, checkRunID, github.UpdateCheckRunOptions{
		Name:        "linter",
		Status:      &checkRunStatus,
		Conclusion:  &conclusion,
		CompletedAt: &t,
		Output: &github.CheckRunOutput{
			Title:       &outputTitle,
			Summary:     &outputSummary,
			Annotations: annotations,
		},
	})
	if err != nil {
		LogError.Errorf("github update check run failed: %v", err)
		return err
	}
	return err
}
