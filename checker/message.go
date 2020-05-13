package checker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/tengattack/unified-ci/store"
	"github.com/tengattack/unified-ci/util"
	"golang.org/x/net/proxy"
	"golang.org/x/sync/errgroup"
)

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
func GenerateAnnotations(ctx context.Context, ref GithubRef, repoPath string, diffs []*diff.FileDiff, lintEnabled LintEnabled,
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
		outputSummary, annotationsArr[0], problemsArr[0], err = lintRepo(ctx, ref, repoPath, diffs, lintEnabled, &bufArr[0])
		return err
	})
	eg.Go(func() error {
		var err error
		// TODO: use ctx
		annotationsArr[1], problemsArr[1], err = lintIndividually(ref, repoPath, diffs, lintEnabled, ignoredPath, &bufArr[1])
		return err
	})
	eg.Go(func() error {
		var err error
		annotationsArr[2], problemsArr[2], err = CheckFileMode(diffs, repoPath, &bufArr[2])
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

func lintRepo(ctx context.Context, ref GithubRef, repoPath string, diffs []*diff.FileDiff, lintEnabled LintEnabled,
	log io.StringWriter) (outputSummary string, annotations []*github.CheckRunAnnotation,
	problems int, err error) {
	annotationLevel := "warning" // TODO: from lint.Severity
	var outputSummaries strings.Builder

	// disable 'xxx' lint check if no 'xxx' files are changed
	disableUnnecessaryLints := func(diffs []*diff.FileDiff, lintEnabled *LintEnabled) {
		goCheck := false
		for _, d := range diffs {
			newName := util.Unquote(d.NewName)
			if strings.HasSuffix(newName, ".go") {
				goCheck = true
				break
			}
		}
		if lintEnabled.Go {
			lintEnabled.Go = goCheck
		}
	}
	disableUnnecessaryLints(diffs, &lintEnabled)

	if lintEnabled.Android {
		log.WriteString(fmt.Sprintf("AndroidLint '%s'\n", repoPath))
		issues, msg, err := AndroidLint(ctx, ref, repoPath)
		if err != nil {
			log.WriteString(fmt.Sprintf("Android lint error: %v\n%s\n", err, msg))
			if msg != "" {
				_, msg = util.Truncated(msg, "... (truncated) ...", 10000)
				err = fmt.Errorf("Android lint error: %v\n```\n%s\n```", err, msg)
			} else {
				err = fmt.Errorf("Android lint error: %v", err)
			}
			return "", nil, 0, err
		}
		if issues != nil {
			for _, d := range diffs {
				fileName, ok := getTrimmedNewName(d)
				if !ok {
					log.WriteString("No need to process " + fileName + "\n")
					continue
				}
				for _, v := range issues.Issues {
					if v.Location.File == fileName {
						startLine := v.Location.Line
						for _, hunk := range d.Hunks {
							if int32(startLine) >= hunk.NewStartLine && int32(startLine) < hunk.NewStartLine+hunk.NewLines {
								var ruleID string
								if v.Category != "" {
									ruleID = v.Category + "." + v.ID
								} else {
									ruleID = v.ID
								}
								comment := fmt.Sprintf("`%s` %d:%d %s",
									ruleID, startLine, v.Location.Column, v.Message)
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
				}
			}
		}
		if issues == nil && msg != "" {
			outputSummaries.WriteString("Android lint error: " + msg)
		}
		log.WriteString(msg + "\n")
	}
	if lintEnabled.APIDoc {
		title := fmt.Sprintf("APIDoc '%s'\n", repoPath)
		log.WriteString(title)
		outputSummaries.WriteString(title)
		var apiDocOutput string
		apiDocOutput, err = APIDoc(ctx, ref, repoPath)
		if err != nil {
			apiDocOutput = fmt.Sprintf("APIDoc error: %v\n", err) + apiDocOutput
			problems++
			err = nil
			// PASS
		}
		log.WriteString(apiDocOutput + "\n") // Add an additional '\n'
		outputSummaries.WriteString(apiDocOutput)
	}
	if lintEnabled.Go {
		log.WriteString(fmt.Sprintf("GolangCILint '%s'\n", repoPath))
		result, msg, err := GolangCILint(ctx, ref, repoPath)
		if err != nil {
			log.WriteString(fmt.Sprintf("GolangCILint error: %v\n%s\n", err, msg))
			if msg != "" {
				_, msg = util.Truncated(msg, "... (truncated) ...", 10000)
				err = fmt.Errorf("GolangCILint error: %v\n```\n%s\n```", err, msg)
			} else {
				err = fmt.Errorf("GolangCILint error: %v", err)
			}
			return "", nil, 0, err
		}
		for _, d := range diffs {
			fileName, ok := getTrimmedNewName(d)
			if !ok {
				log.WriteString("No need to process " + fileName + "\n")
				continue
			}
			if !strings.HasSuffix(fileName, ".go") {
				continue
			}
			for _, v := range result.Issues {
				if fileName == v.Pos.Filename {
					startLine := v.Pos.Line
					for _, hunk := range d.Hunks {
						if int32(startLine) >= hunk.NewStartLine && int32(startLine) < hunk.NewStartLine+hunk.NewLines {
							comment := fmt.Sprintf("%s:%d  %s",
								fileName, startLine, v.Text)
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
			}
		}
		log.WriteString("\n")
	}

	outputSummary = outputSummaries.String()
	return
}

func lintIndividually(ref GithubRef, repoPath string, diffs []*diff.FileDiff, lintEnabled LintEnabled, ignoredPath []string,
	log io.Writer) ([]*github.CheckRunAnnotation, int, error) {
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
		d := d
		fileName, _ := getTrimmedNewName(d)
		if MatchAny(ignoredPath, fileName) {
			continue
		}
		pending <- 0
		eg.Go(func() error {
			defer func() { <-pending }()
			var (
				buf          bytes.Buffer
				annotations_ []*github.CheckRunAnnotation
				problems_    int
			)

			err := handleSingleFile(ref, repoPath, d, lintEnabled, annotationLevel, &buf, &annotations_, &problems_)

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

func handleSingleFile(ref GithubRef, repoPath string, d *diff.FileDiff, lintEnabled LintEnabled, annotationLevel string, log *bytes.Buffer, annotations *[]*github.CheckRunAnnotation, problems *int) error {
	fileName, ok := getTrimmedNewName(d)
	if !ok {
		log.WriteString("No need to process " + fileName + "\n")
		return nil
	}
	log.WriteString(fmt.Sprintf("Checking '%s'\n", fileName))

	var lints []LintMessage
	var lintErr error
	if lintEnabled.MD && strings.HasSuffix(fileName, ".md") {
		log.WriteString(fmt.Sprintf("Markdown '%s'\n", fileName))
		rps, out, err := remark(ref, fileName, repoPath)
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
		lints, lintErr = CPPLint(ref, fileName, repoPath)
	} else if isOC(fileName) {
		if lintEnabled.OC {
			log.WriteString(fmt.Sprintf("OCLint '%s'\n", fileName))
			lints, lintErr = OCLint(context.TODO(), ref, fileName, repoPath)
		}
		if lintEnabled.ClangLint {
			log.WriteString(fmt.Sprintf("ClangLint '%s'\n", fileName))
			lintsDiff, err := ClangLint(context.TODO(), ref, repoPath, filepath.Join(repoPath, fileName))
			if err != nil {
				return err
			}
			pickDiffLintMessages(lintsDiff, d, annotations, problems, log, fileName)
		}
	} else if strings.HasSuffix(fileName, ".kt") {
		lints, lintErr = Ktlint(context.TODO(), ref, fileName, repoPath)
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
		lints, errlog, lintErr = PHPLint(ref, filepath.Join(repoPath, fileName), repoPath)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	} else if lintEnabled.TypeScript && (strings.HasSuffix(fileName, ".ts") ||
		strings.HasSuffix(fileName, ".tsx")) {
		log.WriteString(fmt.Sprintf("TSLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = TSLint(ref, filepath.Join(repoPath, fileName), repoPath)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	} else if lintEnabled.SCSS && (strings.HasSuffix(fileName, ".scss") ||
		strings.HasSuffix(fileName, ".css")) {
		log.WriteString(fmt.Sprintf("SCSSLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = SCSSLint(ref, filepath.Join(repoPath, fileName), repoPath)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	} else if lintEnabled.JS != "" && strings.HasSuffix(fileName, ".js") {
		log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = ESLint(ref, filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
		if errlog != "" {
			log.WriteString(errlog + "\n")
		}
	} else if lintEnabled.ES != "" && (strings.HasSuffix(fileName, ".es") ||
		strings.HasSuffix(fileName, ".esx") || strings.HasSuffix(fileName, ".jsx")) {
		log.WriteString(fmt.Sprintf("ESLint '%s'\n", fileName))
		var errlog string
		lints, errlog, lintErr = ESLint(ref, filepath.Join(repoPath, fileName), repoPath, lintEnabled.ES)
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
		lints2, errlog, err := ESLint(ref, filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
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
func HandleMessage(ctx context.Context, message string) error {
	// 限制总时长为一个小时
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()

	s := strings.Split(message, "/")

	var repository, pull, commitSha string

	if len(s) != 6 || (s[2] != "pull" && s[2] != "tree") || s[4] != "commits" {
		LogAccess.Warnf("malformed message: %s", message)
		return nil
	}

	checkType := s[2]
	repository, pull, commitSha = s[0]+"/"+s[1], s[3], s[5]
	prNum := 0
	if checkType == "tree" {
		// branchs
		LogAccess.Infof("Start handling %s/tree/%s", repository, pull)
	} else {
		// pulls
		var err error
		prNum, err = strconv.Atoi(pull)
		if err != nil {
			LogAccess.Warnf("malformed message: %s", message)
			return nil
		}
		LogAccess.Infof("Start handling %s/pull/%s", repository, pull)
	}

	// ref to be checked in the owner/repo
	ref := GithubRef{
		owner: s[0],
		repo:  s[1],
		Sha:   commitSha,
	}
	if checkType == "tree" {
		ref.checkType = CheckTypeBranch
		ref.checkRef = pull
	} else {
		ref.checkType = CheckTypePRHead
		ref.checkRef = "pr/" + pull
	}

	targetURL := ""
	if len(Conf.Core.CheckLogURI) > 0 {
		targetURL = Conf.Core.CheckLogURI + repository + "/" + ref.Sha + ".log"
	}

	repoLogsPath := filepath.Join(Conf.Core.LogsDir, repository)
	_ = os.MkdirAll(repoLogsPath, os.ModePerm)

	log, err := os.Create(filepath.Join(repoLogsPath, fmt.Sprintf("%s.log", ref.Sha)))
	if err != nil {
		return err
	}

	var client *github.Client
	var gpull *github.PullRequest

	// Wrap the shared transport for use with the integration ID authenticating with installation ID.
	// TODO: add installation ID to db
	installationID, ok := Conf.GitHub.Installations[ref.owner]
	if ok {
		var tr http.RoundTripper
		if Conf.Core.Socks5Proxy != "" {
			dialSocksProxy, err := proxy.SOCKS5("tcp", Conf.Core.Socks5Proxy, nil, proxy.Direct)
			if err != nil {
				msg := "Setup proxy failed: " + err.Error()
				// close log manually
				log.WriteString(msg + "\n")
				log.Close()
				return errors.New(msg)
			}
			tr = &http.Transport{Dial: dialSocksProxy.Dial}
		} else {
			tr = http.DefaultTransport
		}
		tr, err := ghinstallation.NewKeyFromFile(tr,
			Conf.GitHub.AppID, installationID, Conf.GitHub.PrivateKey)
		if err != nil {
			msg := "Load private key failed: " + err.Error()
			// close log manually
			log.WriteString(msg + "\n")
			log.Close()
			return errors.New(msg)
		}

		// TODO: refine code
		client = github.NewClient(&http.Client{Transport: tr})
	} else {
		msg := "Installation ID not found, owner: " + ref.owner
		// close log manually
		log.WriteString(msg + "\n")
		log.Close()
		return errors.New(msg)
	}

	defer func() {
		if err != nil {
			log.WriteString("Handle message failed: " + err.Error() + "\n")
		} else {
			log.WriteString("done.")
			LogAccess.Infof("Finish message: %s", message)
		}
		log.Close()
	}()

	log.WriteString(UserAgent() + " Date: " + time.Now().Format(time.RFC1123) + "\n\n")

	if ref.IsBranch() {
		log.WriteString(fmt.Sprintf("Start fetching %s/tree/%s\n", repository, pull))
	} else {
		log.WriteString(fmt.Sprintf("Start fetching %s/pull/%s\n", repository, pull))

		exist, err := util.SearchGithubPR(ctx, client, repository, commitSha)
		if err != nil {
			err = fmt.Errorf("SearchGithubPR error: %v", err)
			return err
		}
		if exist == 0 {
			log.WriteString(fmt.Sprintf("commit: %s no longer exists.\n", commitSha))
			return nil
		}

		gpull, err = GetGithubPull(ctx, client, ref.owner, ref.repo, prNum)
		if err != nil {
			err = fmt.Errorf("GetGithubPull error: %v", err)
			return err
		}
		if gpull.GetState() != "open" {
			log.WriteString("PR " + gpull.GetState() + ".\n")
			return nil
		}
	}

	err = ref.UpdateState(client, AppName, "pending", targetURL, "checking")
	if err != nil {
		err = fmt.Errorf("Update pull request status error: %v", err)
		return err
	}

	repoPath := filepath.Join(Conf.Core.WorkDir, repository)
	_ = os.MkdirAll(repoPath, os.ModePerm)

	parser := NewShellParser(repoPath, ref)
	words, err := parser.Parse(Conf.Core.GitCommand)
	if err != nil {
		err = fmt.Errorf("parse git command error: %v", err)
		return err
	}

	log.WriteString("$ git init\n")
	gitCmds := make([]string, len(words))
	copy(gitCmds, words)
	gitCmds = append(gitCmds, "init")
	cmd := exec.CommandContext(ctx, gitCmds[0], gitCmds[1:]...)
	cmd.Dir = repoPath
	err = cmd.Run()
	if err != nil {
		return err
	}

	installationToken, _, err := util.JWTClient.Apps.CreateInstallationToken(ctx, int64(installationID), nil)
	if err != nil {
		return err
	}

	var cloneURL string
	if ref.IsBranch() {
		// branchs
		// TODO: using GetBranch api
		cloneURL = "https://github.com/" + ref.owner + "/" + ref.repo + ".git"
	} else {
		// pulls
		cloneURL = gpull.GetBase().GetRepo().GetCloneURL()
	}
	originURL, err := url.Parse(cloneURL) // e.g. https://github.com/octocat/Hello-World.git
	if err != nil {
		return err
	}
	originURL.User = url.UserPassword("x-access-token", installationToken.GetToken())

	fetchURL := originURL.String()
	if ref.IsBranch() {
		localBranch := pull

		log.WriteString("$ git fetch -f -u " + cloneURL +
			fmt.Sprintf(" %s:%s\n", pull, localBranch))
		gitCmds = make([]string, len(words))
		copy(gitCmds, words)
		gitCmds = append(gitCmds, "fetch", "-f", "-u", fetchURL,
			fmt.Sprintf("%s:%s", pull, localBranch))
		cmd = exec.CommandContext(ctx, gitCmds[0], gitCmds[1:]...)
	} else {
		localBranch := fmt.Sprintf("pull-%d", prNum)

		// git fetch -f -u https://x-access-token:token@github.com/octocat/Hello-World.git pull/%d/head:pull-%d
		// -u option can be used to bypass the restriction which prevents git from fetching into current branch:
		// link: https://stackoverflow.com/a/32561463/4213218
		log.WriteString("$ git fetch -f -u " + cloneURL +
			fmt.Sprintf(" pull/%d/head:%s\n", prNum, localBranch))
		gitCmds = make([]string, len(words))
		copy(gitCmds, words)
		gitCmds = append(gitCmds, "fetch", "-f", "-u", fetchURL,
			fmt.Sprintf("pull/%d/head:%s", prNum, localBranch))
		cmd = exec.CommandContext(ctx, gitCmds[0], gitCmds[1:]...)
	}
	cmd.Dir = repoPath
	cmd.Stdout = log
	cmd.Stderr = log
	err = cmd.Run()
	if err != nil {
		return err
	}

	// git checkout -f <commits>/<branch>
	log.WriteString("$ git checkout -f " + ref.Sha + "\n")
	gitCmds = make([]string, len(words))
	copy(gitCmds, words)
	gitCmds = append(gitCmds, "checkout", "-f", ref.Sha)
	cmd = exec.CommandContext(ctx, gitCmds[0], gitCmds[1:]...)
	cmd.Dir = repoPath
	cmd.Stdout = log
	cmd.Stderr = log
	err = cmd.Run()
	if err != nil {
		return err
	}

	var diffs []*diff.FileDiff
	if checkType == "pull" {
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
		out, err := GetGithubPullDiff(ctx, client, ref.owner, ref.repo, prNum)
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

		err = LabelPRSize(ctx, client, ref, prNum, diffs)
		if err != nil {
			log.WriteString("Label PR error: " + err.Error() + "\n")
			LogError.Errorf("Label PR error: %v", err)
			// PASS
		}
	}

	lintEnabled := LintEnabled{}
	lintEnabled.Init(repoPath)

	repoConf, err := readProjectConfig(repoPath)
	if err != nil {
		err = fmt.Errorf("ReadProjectConfig error: %v", err)
		outputTitle := "wrong ci config"
		log.WriteString(err.Error() + "\n")
		if ref.IsBranch() {
			// Update state to error
			erro := ref.UpdateState(client, AppName, "error", targetURL, outputTitle)
			if erro != nil {
				LogError.Errorf("Failed to update state to error: %v", erro)
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

	mark := '✔'
	sumCount := failedLints + failedTests
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
		err = ref.UpdateState(client, AppName, "error", targetURL, outputSummary)
	} else {
		// update PR state
		outputSummary = "The check succeed!"
		err = ref.UpdateState(client, AppName, "success", targetURL, outputSummary)
	}
	if err != nil {
		log.WriteString("UpdateState error: " + err.Error() + "\n")
		LogError.Errorf("UpdateState error: %v", err)
		// PASS
	}

	if checkType == "pull" {
		// create review
		if sumCount > 0 {
			comment := fmt.Sprintf("**lint**: %d problem(s) found.\n", failedLints)
			if !noTest {
				comment += fmt.Sprintf("**test**: %d problem(s) found.\n\n", failedTests)
				comment += testMsg
			}
			err = ref.CreateReview(client, prNum, "REQUEST_CHANGES", comment, nil)
		} else {
			comment := "**check**: no problems found.\n"
			if !noTest {
				comment += ("\n" + testMsg)
			}
			err = ref.CreateReview(client, prNum, "APPROVE", comment, nil)
		}
		if err != nil {
			err = fmt.Errorf("CreateReview error: %v", err)
		}
	}
	return err
}

// TODO: add test
func checkLints(ctx context.Context, client *github.Client, gpull *github.PullRequest, ref GithubRef, targetURL string,
	repoPath string, diffs []*diff.FileDiff, lintEnabled LintEnabled, ignoredPath []string, log *os.File) (problems int, err error) {

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
		LogAccess.Warn("Too many annotations to push them all at once. Only 50 annotations will be pushed right now.")
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
		if !MatchAny(ignoredPath, a.GetPath()) {
			filteredAnnotations = append(filteredAnnotations, a)
		}
	}
	return filteredAnnotations, len(annotations) - len(filteredAnnotations)
}

func checkTests(ctx context.Context, repoPath string, tests map[string]goTestsConfig,
	client *github.Client, gpull *github.PullRequest, ref GithubRef,
	targetURL string, log *os.File) (failedTests, passedTests, errTests int, testMsg string) {

	t := &testReporter{
		RepoPath:  repoPath,
		Client:    client,
		Pull:      gpull,
		Ref:       ref,
		TargetURL: targetURL,
	}
	t.LogDivider = NewLogDivider(len(tests) > 1, log)
	var headCoverage sync.Map
	failedTests, passedTests, errTests = runTests(ctx, tests, t, &headCoverage)

	if !ref.IsBranch() {
		// compare test coverage with base
		baseSHA, err := util.GetBaseSHA(ctx, client, ref.owner, ref.repo, gpull.GetNumber())
		if err != nil {
			msg := fmt.Sprintf("Cannot get BaseSHA: %v\n", err)
			LogError.Error(msg)
			log.WriteString(msg)
			return
		}
		baseSavedRecords, baseTestsNeedToRun := loadBaseFromStore(ref, baseSHA, tests, log)
		var baseCoverage sync.Map
		_ = findBaseCoverage(ctx, baseSavedRecords, baseTestsNeedToRun, repoPath, baseSHA, gpull, ref, log, &baseCoverage)
		testMsg = util.DiffCoverage(&headCoverage, &baseCoverage)
	}
	return
}

type testRunner interface {
	Run(ctx context.Context, testName string, testConfig goTestsConfig) (string, error)
}

type testReporter struct {
	*LogDivider

	RepoPath  string
	Client    *github.Client
	Pull      *github.PullRequest
	Ref       GithubRef
	TargetURL string
}

func (t *testReporter) Run(ctx context.Context, testName string, testConfig goTestsConfig) (reportMessage string, err error) {
	t.Log(func(w io.Writer) {
		reportMessage, err = ReportTestResults(ctx, testName, t.RepoPath, testConfig.Cmds, testConfig.Coverage, t.Client, t.Pull,
			t.Ref, t.TargetURL, w)
	})
	return
}

func runTests(ctx context.Context, tests map[string]goTestsConfig, t testRunner, coverageMap *sync.Map) (failedTests, passedTests, errTests int) {
	maxPendingTests := Conf.Concurrency.Test
	if maxPendingTests < 1 {
		maxPendingTests = 1
	}
	pendingTests := make(chan int, maxPendingTests)

	var (
		wg          sync.WaitGroup
		errCount    int64
		failedCount int64
		passedCount int64
	)
	for k, v := range tests {
		testName := k
		testConfig := v

		if isEmptyTest(testConfig.Cmds) {
			continue
		}

		pendingTests <- 0
		wg.Add(1)
		go func() {
			defer func() {
				if info := recover(); info != nil {
					atomic.AddInt64(&errCount, 1)
				}
				wg.Done()
				<-pendingTests
			}()
			percentage, err := t.Run(ctx, testName, testConfig)
			if testConfig.Coverage != "" {
				coverageMap.Store(testName, percentage)
			}
			if err != nil {
				if _, ok := err.(*testNotPass); ok {
					atomic.AddInt64(&failedCount, 1)
				} else {
					atomic.AddInt64(&errCount, 1)
				}
			} else {
				atomic.AddInt64(&passedCount, 1)
			}
		}()
	}

	wg.Wait()
	failedTests = int(failedCount)
	passedTests = int(passedCount)
	errTests = int(errCount)
	return
}

func loadBaseFromStore(ref GithubRef, baseSHA string, tests map[string]goTestsConfig,
	log io.Writer) ([]store.CommitsInfo, map[string]goTestsConfig) {
	baseSavedRecords, err := store.ListCommitsInfo(ref.owner, ref.repo, baseSHA)
	if err != nil {
		msg := fmt.Sprintf("Failed to load base info: %v\n", err)
		LogError.Error(msg)
		io.WriteString(log, msg)
		// PASS
	}

	baseTestsNeedToRun := make(map[string]goTestsConfig)
	for testName, testCfg := range tests {
		found := false
		if testCfg.Coverage == "" {
			// no need to run in base as it has no coverage requirements
			continue
		}
		for _, v := range baseSavedRecords {
			if testName == v.Test {
				found = true
				break
			}
		}
		if !found {
			baseTestsNeedToRun[testName] = testCfg
		}
	}
	io.WriteString(log,
		fmt.Sprintf("baseSavedRecords: %d, baseTestsNeedToRun: %d\n\n", len(baseSavedRecords), len(baseTestsNeedToRun)))
	return baseSavedRecords, baseTestsNeedToRun
}

func findBaseCoverage(ctx context.Context, baseSavedRecords []store.CommitsInfo, baseTestsNeedToRun map[string]goTestsConfig, repoPath string,
	baseSHA string, gpull *github.PullRequest, ref GithubRef, log io.Writer, baseCoverage *sync.Map) error {
	for _, v := range baseSavedRecords {
		if v.Coverage == nil {
			baseCoverage.Store(v.Test, "nil")
		} else {
			baseCoverage.Store(v.Test, util.FormatFloatPercent(*v.Coverage))
		}
	}

	parser := NewShellParser(repoPath, ref)
	words, err := parser.Parse(Conf.Core.GitCommand)
	if err != nil {
		err = fmt.Errorf("parse git command error: %v", err)
		return err
	}

	if len(baseTestsNeedToRun) > 0 {
		io.WriteString(log, "$ git checkout -f "+baseSHA+"\n")
		gitCmds := make([]string, len(words))
		copy(gitCmds, words)
		gitCmds = append(gitCmds, "checkout", "-f", baseSHA)
		cmd := exec.Command(gitCmds[0], gitCmds[1:]...)
		cmd.Dir = repoPath
		cmd.Stdout = log
		cmd.Stderr = log
		err := cmd.Run()
		if err != nil {
			msg := fmt.Sprintf("Failed to checkout to base: %v\n", err)
			LogError.Error(msg)
			io.WriteString(log, msg)
			return err
		}

		t := &baseTestAndSave{
			Ref:      ref,
			BaseSHA:  baseSHA,
			RepoPath: repoPath,
			Pull:     gpull,
		}
		t.LogDivider = NewLogDivider(len(baseTestsNeedToRun) > 1, log)
		runTests(ctx, baseTestsNeedToRun, t, baseCoverage)

		io.WriteString(log, "$ git checkout -f "+ref.Sha+"\n")
		gitCmds = make([]string, len(words))
		copy(gitCmds, words)
		gitCmds = append(gitCmds, "checkout", "-f", ref.Sha)
		cmd = exec.Command(gitCmds[0], gitCmds[1:]...)
		cmd.Dir = repoPath
		cmd.Stdout = log
		cmd.Stderr = log
		err = cmd.Run()
		if err != nil {
			msg := fmt.Sprintf("Failed to checkout back: %v\n", err)
			LogError.Error(msg)
			io.WriteString(log, msg)
			return err
		}
	}
	return nil
}

type baseTestAndSave struct {
	*LogDivider

	Ref      GithubRef
	BaseSHA  string
	RepoPath string
	Pull     *github.PullRequest
}

func (t *baseTestAndSave) Run(ctx context.Context, testName string, testConfig goTestsConfig) (string, error) {
	var reportMessage string
	t.Log(func(w io.Writer) {
		ref := t.Ref
		ref.Sha = t.BaseSHA
		if ref.checkType == CheckTypePRHead {
			ref.checkType = CheckTypePRBase
		}

		_, reportMessage, _ = testAndSaveCoverage(ctx, ref,
			testName, testConfig.Cmds, testConfig.Coverage, t.RepoPath, t.Pull, true, w)
	})
	return reportMessage, nil
}
