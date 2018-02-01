package checker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sourcegraph.com/sourcegraph/go-diff/diff"
)

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
		Repo: repository,
		Sha:  commits,
	}
	targetURL := ""
	if len(Conf.Core.CheckLogURI) > 0 {
		targetURL = Conf.Core.CheckLogURI + repository + "/" + commits + ".log"
	}
	err := ref.UpdateState("lint", "pending", targetURL,
		"checking")
	if err != nil {
		LogAccess.Error("Update pull request status error: " + err.Error())
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

	_, err = GetGithubPull(repository, pull)
	if err != nil {
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

	log.WriteString("$ git remote add origin " +
		fmt.Sprintf("git@github.com:%s.git\n", repository))
	cmd = exec.Command("git", "remote", "add", "origin",
		fmt.Sprintf("git@github.com:%s.git", repository))
	cmd.Dir = repoPath
	err = cmd.Run()
	if err != nil {
		// return err
	}

	// git fetch -f origin pull/XX/head:pull-XX
	log.WriteString("$ git fetch -f origin " +
		fmt.Sprintf("pull/%s/head:pull-%s\n", pull, pull))
	cmd = exec.Command("git", "fetch", "-f", "origin",
		fmt.Sprintf("pull/%s/head:pull-%s", pull, pull))
	cmd.Dir = repoPath
	cmd.Stdout = log
	cmd.Stderr = log
	err = cmd.Run()
	if err != nil {
		return err
	}

	// git checkout <commits>
	log.WriteString("$ git checkout " + commits + "\n")
	cmd = exec.Command("git", "checkout", commits)
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

	comments := []GithubRefComment{}
	problems := 0
	for _, d := range diffs {
		if strings.HasPrefix(d.NewName, "b/") {
			fileName := d.NewName[2:]
			log.WriteString(fmt.Sprintf("Checking '%s'\n", fileName))
			if strings.HasSuffix(fileName, ".php") {
				log.WriteString(fmt.Sprintf("PHPLint '%s'\n", fileName))
				lints, err := PHPLint(filepath.Join(repoPath, fileName))
				if err != nil {
					return err
				}
				for _, hunk := range d.Hunks {
					if hunk.NewLines > 0 {
						lines := strings.Split(string(hunk.Body), "\n")
						for _, lint := range lints {
							if lint.Line >= int(hunk.NewStartLine) &&
								lint.Line < int(hunk.NewStartLine+hunk.NewLines) {
								lineNum := 0
								i := 0
								for ; i < len(lines); i++ {
									if len(lines[i]) <= 0 || lines[i][0] != '-' {
										if lineNum <= 0 {
											lineNum = int(hunk.NewStartLine)
										} else {
											lineNum++
										}
									}
									if lineNum >= lint.Line {
										break
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
				}
				log.WriteString("\n")
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
