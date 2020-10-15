package lint

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/go-github/github"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/util"
	"golang.org/x/sync/errgroup"
)

func LintRepo(ctx context.Context, ref common.GithubRef, repoPath string, diffs []*diff.FileDiff, lintEnabled LintEnabled,
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
				fileName, ok := util.GetTrimmedNewName(d)
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
			fileName, ok := util.GetTrimmedNewName(d)
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

func LintIndividually(ctx context.Context, ref common.GithubRef, repoPath string, diffs []*diff.FileDiff, lintEnabled LintEnabled, ignoredPath []string,
	log io.Writer) ([]*github.CheckRunAnnotation, int, error) {
	annotationLevel := "warning" // TODO: from lint.Severity
	maxPending := common.Conf.Concurrency.Lint
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
		fileName, _ := util.GetTrimmedNewName(d)
		if util.MatchAny(ignoredPath, fileName) {
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

func findTsConfig(fileName string, repoPath string) string {
	const tsConfig = "tsconfig.json"
	fullPath := filepath.Join(repoPath, fileName)
	dir := filepath.Dir(fullPath)
	for len(dir) >= len(repoPath) {
		tryTsConfigFile := filepath.Join(dir, tsConfig)
		if util.FileExists(tryTsConfigFile) {
			return tryTsConfigFile
		}
		if dir == repoPath {
			break
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func handleSingleFile(ref common.GithubRef, repoPath string, d *diff.FileDiff, lintEnabled LintEnabled, annotationLevel string, log *bytes.Buffer, annotations *[]*github.CheckRunAnnotation, problems *int) error {
	fileName, ok := util.GetTrimmedNewName(d)
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
	} else if strings.HasSuffix(fileName, ".ts") ||
		strings.HasSuffix(fileName, ".tsx") {
		var errlog string
		if lintEnabled.TypeScript {
			log.WriteString(fmt.Sprintf("TSLint '%s'\n", fileName))
			tsConfigFile := findTsConfig(fileName, repoPath)
			if tsConfigFile != "" {
				lints, errlog, lintErr = TSLint(ref,
					filepath.Join(repoPath, fileName), tsConfigFile, repoPath)
			} else {
				// 如果没有 tslint 的配置文件，则用 eslint 检查
				lints, errlog, lintErr = ESLint(ref, filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
			}
			// TypeScript 未开启，尝试用 eslint 检查
		} else if lintEnabled.JS != "" {
			lints, errlog, lintErr = ESLint(ref, filepath.Join(repoPath, fileName), repoPath, lintEnabled.JS)
		}
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

const (
	fileModeCheckNormal      = "Normal file permission should be 0644"
	fileModeCheckExecutable  = "Executable file permission should be 0755"
	fileModeCheckShellScript = "Shell script file permission should be 0755"
	shebangCheckShellScript  = "Shell script file should start with #!"
)

// LintFileMode checks repo's files' mode
func LintFileMode(ctx context.Context, ref common.GithubRef, repoPath string, diffs []*diff.FileDiff, log io.StringWriter) ([]*github.CheckRunAnnotation, int, error) {
	startLine := 1
	endLine := 1
	annotationLevel := "warning"

	problem := 0
	annotations := make([]*github.CheckRunAnnotation, 0, len(diffs))
	for _, d := range diffs {
		fileName, _ := util.GetTrimmedNewName(d)
		filePath := filepath.Join(repoPath, fileName)
		if fileName == "/dev/null" {
			continue
		}
		mode, _ := util.ParseFileModeInDiff(d.Extended)
		if mode == 0 {
			log.WriteString(fmt.Sprintf("Failed to parse file mode of %s.\n", fileName))
			continue
		}
		comment := ""
		switch strings.ToLower(filepath.Ext(fileName)) {
		case ".sh":
			if mode != 0755 {
				problem++
				comment = fileModeCheckShellScript
			} else {
				lines, err := util.HeadFile(filePath, 1)
				if err != nil {
					log.WriteString(fmt.Sprintf("Failed to read %s: %v\n", fileName, err))
					continue
				}
				if len(lines) > 0 {
					if !strings.HasPrefix(lines[0], "#!") {
						problem++
						comment = shebangCheckShellScript
					}
				}
			}
		case ".js", ".py":
			lines, err := util.HeadFile(filePath, 1)
			if err != nil {
				log.WriteString(fmt.Sprintf("Failed to read %s: %v\n", fileName, err))
				continue
			}
			if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
				if mode != 0755 {
					problem++
					comment = fileModeCheckExecutable
				}
			} else {
				if mode != 0644 {
					problem++
					comment = fileModeCheckNormal
				}
			}
		default:
			if mode != 0644 {
				problem++
				comment = fileModeCheckNormal
			}
		}
		if comment != "" {
			annotations = append(annotations, &github.CheckRunAnnotation{
				Path:            &fileName,
				StartLine:       &startLine,
				EndLine:         &endLine,
				AnnotationLevel: &annotationLevel,
				Message:         &comment,
			})
		}
	}

	return annotations, problem, nil
}
