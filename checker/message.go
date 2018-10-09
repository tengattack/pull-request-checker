package checker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sourcegraph.com/sourcegraph/go-diff/diff"
)

// GenerateComments generate github comments from github diffs and lint option
func GenerateComments(repoPath string, diffs []*diff.FileDiff, lintEnabled *LintEnabled, log *os.File) ([]GithubRefComment, int, error) {
	comments := []GithubRefComment{}
	var err error
	problems := 0
	for _, d := range diffs {
		if strings.HasPrefix(d.NewName, "b/") {
			fileName := d.NewName[2:]
			log.WriteString(fmt.Sprintf("Checking '%s'\n", fileName))
			var lints []LintMessage
			if lintEnabled.Go && strings.HasSuffix(fileName, ".go") {
				log.WriteString(fmt.Sprintf("Goreturns '%s'\n", fileName))
				lintsGoreturns, err := Goreturns(filepath.Join(repoPath, fileName), repoPath)
				if err != nil {
					return nil, 0, err
				}
				for _, lint := range lintsGoreturns {
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
				log.WriteString(fmt.Sprintf("Golint '%s'\n", fileName))
				lints, err = Golint(filepath.Join(repoPath, fileName), repoPath)
			} else if lintEnabled.PHP && strings.HasSuffix(fileName, ".php") {
				log.WriteString(fmt.Sprintf("PHPLint '%s'\n", fileName))
				lints, err = PHPLint(filepath.Join(repoPath, fileName), repoPath)
			} else if lintEnabled.TypeScript && (strings.HasSuffix(fileName, ".ts") ||
				strings.HasSuffix(fileName, ".tsx")) {
				log.WriteString(fmt.Sprintf("TSLint '%s'\n", fileName))
				lints, err = TSLint(filepath.Join(repoPath, fileName), repoPath)
			} else if lintEnabled.SCSS && (strings.HasSuffix(fileName, ".scss") ||
				strings.HasSuffix(fileName, ".css")) {
				log.WriteString(fmt.Sprintf("SCSSLint '%s'\n", fileName))
				lints, err = SCSSLint(filepath.Join(repoPath, fileName), repoPath)
			} else if lintEnabled.JS != "" && strings.HasSuffix(fileName, ".js") {
				log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
				lints, err = ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
			} else if lintEnabled.ES != "" && (strings.HasSuffix(fileName, ".es") ||
				strings.HasSuffix(fileName, ".esx") || strings.HasSuffix(fileName, ".jsx")) {
				log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
				lints, err = ESLint(filepath.Join(repoPath, fileName), repoPath, lintEnabled.ES)
			}
			if err != nil {
				return nil, 0, err
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
					// end for
				}
			}
			log.WriteString("\n")
		}
	}
	return comments, problems, nil
}

// HandleMessage handles message
func HandleMessage(message string) error {
	s := strings.Split(message, "/")
	if len(s) != 6 || s[2] != "pull" || s[4] != "commits" {
		LogAccess.Warnf("malfromed message: %s", message)
		return nil
	}

	repository, pull, commits := s[0]+"/"+s[1], s[3], s[5]
	LogAccess.Infof("Start fetching %s/pull/%s", repository, pull)

	ref := GithubRef{
		RepoName: repository,
		Sha:      commits,
	}
	targetURL := ""
	if len(Conf.Core.CheckLogURI) > 0 {
		targetURL = Conf.Core.CheckLogURI + repository + "/" + commits + ".log"
	}

	repoLogsPath := filepath.Join(Conf.Core.LogsDir, repository)
	os.MkdirAll(repoLogsPath, os.ModePerm)

	log, err := os.Create(filepath.Join(repoLogsPath, fmt.Sprintf("%s.log", commits)))
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			log.WriteString("error: " + err.Error() + "\n")
		}
		log.Close()
	}()

	log.WriteString("Pull Request Checker/" + GetVersion() + "\n\n")
	log.WriteString(fmt.Sprintf("Start fetching %s/pull/%s\n", repository, pull))

	gpull, err := GetGithubPull(repository, pull)
	if err != nil {
		return err
	}
	if gpull.State != "open" {
		log.WriteString("PR " + gpull.State + ".")
		return nil
	}

	err = ref.UpdateState("lint", "pending", targetURL,
		"checking")
	if err != nil {
		LogAccess.Error("Update pull request status error: " + err.Error())
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
	log.WriteString("$ git checkout -f " + commits + "\n")
	cmd = exec.Command("git", "checkout", "-f", commits)
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
	log.WriteString("\n---------------------\n")
	log.WriteString(string(out))
	log.WriteString("\n---------------------\n")
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

	comments, problems, err := GenerateComments(repoPath, diffs, &lintEnabled, log)
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

	if problems > 0 {
		comment := fmt.Sprintf("**lint**: %d problem(s) found.", problems)
		err = ref.CreateReview(pull, "REQUEST_CHANGES", comment, comments)
		if err != nil {
			log.WriteString("error: " + err.Error() + "\n")
		}
		err = ref.UpdateState("lint", "error", targetURL,
			fmt.Sprintf("The lint check failed! %d problem(s) found.", problems))
	} else {
		// err = ref.CreateReview(pull, "APPROVE", "**lint**: no problems found.", nil)
		// if err != nil {
		// 	log.WriteString("error: " + err.Error() + "\n")
		// }
		err = ref.UpdateState("lint", "success", targetURL,
			"The lint check succeed!")
	}
	if err == nil {
		log.WriteString("done.")
	}

	return err
}
