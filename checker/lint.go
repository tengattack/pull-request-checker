package checker

import (
	"encoding/json"
	"fmt"
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/sqs/goreturns/returns"
	"golang.org/x/lint"
	"golang.org/x/tools/imports"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

const golintMinConfidenceDefault = 0.8 // 0 ~ 1
const (
	severityLevelOff = iota
	severityLevelWarning
	severityLevelError
)
const (
	ruleGolint    = "golint"
	ruleGoreturns = "goreturns"
)

// LintEnabled list enabled linter
type LintEnabled struct {
	Go         bool
	PHP        bool
	TypeScript bool
	SCSS       bool
	JS         string
	ES         string
}

// LintMessage is a single lint message for PHPLint
type LintMessage struct {
	RuleID     string `json:"ruleId"`
	Severity   int    `json:"severity"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Message    string `json:"message"`
	SourceCode string `json:"sourceCode,omitempty"`
}

// LintResult is a single lint result for PHPLint
type LintResult struct {
	FilePath string        `json:"filePath"`
	Messages []LintMessage `json:"messages"`
}

// TSLintResult is a single lint result for TSLint
type TSLintResult struct {
	Name          string         `json:"name"`
	RuleName      string         `json:"ruleName"`
	RuleSeverity  string         `json:"ruleSeverity"`
	Failure       string         `json:"failure"`
	StartPosition TSLintPosition `json:"startPosition"`
	EndPosition   TSLintPosition `json:"endPosition"`
}

// TSLintPosition is the source code position
type TSLintPosition struct {
	Character int `json:"character"`
	Line      int `json:"line"`
	Position  int `json:"position"`
}

// SCSSLintResult is a single lint result for SCSSLint
type SCSSLintResult struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Length   int    `json:"length"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Linter   string `json:"linter"`
}

// LintSeverity is the map of rule severity name
var LintSeverity map[string]int

func init() {
	LintSeverity = map[string]int{
		"off":     severityLevelOff,
		"warning": severityLevelWarning,
		"error":   severityLevelError,
	}
}

// Init default LintEnabled struct
func (lintEnabled *LintEnabled) Init(cwd string) {

	// reset to defaults
	lintEnabled.Go = true
	lintEnabled.PHP = true
	lintEnabled.TypeScript = false
	lintEnabled.SCSS = false
	lintEnabled.JS = ""
	lintEnabled.ES = ""

	if _, err := os.Stat(filepath.Join(cwd, "tslint.json")); err == nil {
		lintEnabled.TypeScript = true
	}
	if _, err := os.Stat(filepath.Join(cwd, ".scss-lint.yml")); err == nil {
		lintEnabled.SCSS = true
	}
	if _, err := os.Stat(filepath.Join(cwd, ".eslintrc")); err == nil {
		lintEnabled.ES = filepath.Join(cwd, ".eslintrc")
	}
	if _, err := os.Stat(filepath.Join(cwd, ".eslintrc.js")); err == nil {
		lintEnabled.JS = filepath.Join(cwd, ".eslintrc.js")
		if lintEnabled.ES == "" {
			lintEnabled.ES = lintEnabled.JS
		}
	} else {
		lintEnabled.JS = lintEnabled.ES
	}
}

// PHPLint lints the php files
func PHPLint(fileName, cwd string) ([]LintMessage, error) {
	var cmd *exec.Cmd
	if len(Conf.Core.PHPLintConfig) > 0 {
		cmd = exec.Command("php", Conf.Core.PHPLint, "-f", "json", "-c", Conf.Core.PHPLintConfig, fileName)
	} else {
		cmd = exec.Command("php", Conf.Core.PHPLint, "-f", "json", fileName)
	}
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	LogAccess.Debugf("PHPLint Result:\n%s", out)

	var results []LintResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		return nil, err
	}

	if len(results) <= 0 {
		return []LintMessage{}, nil
	}
	return results[0].Messages, nil
}

// ESLint lints the js, jsx, es, esx files
func ESLint(fileName, cwd, eslintrc string) ([]LintMessage, error) {
	var cmd *exec.Cmd
	if eslintrc != "" {
		cmd = exec.Command(Conf.Core.ESLint, "-c", eslintrc, "-f", "json", fileName)
	} else {
		cmd = exec.Command(Conf.Core.ESLint, "-f", "json", fileName)
	}
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, err
		}
	}

	LogAccess.Debugf("TSLint Result:\n%s", out)

	var results []LintResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		return nil, err
	}

	if len(results) <= 0 {
		return []LintMessage{}, nil
	}
	return results[0].Messages, nil
}

// TSLint lints the ts and tsx files
func TSLint(fileName, cwd string) ([]LintMessage, error) {
	var cmd *exec.Cmd
	cmd = exec.Command(Conf.Core.TSLint, "--format", "json", fileName)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, err
		}
	}

	LogAccess.Debugf("TSLint Result:\n%s", out)

	var results []TSLintResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		return nil, err
	}

	if len(results) <= 0 {
		return []LintMessage{}, nil
	}

	var messages []LintMessage
	messages = make([]LintMessage, len(results))
	for i, lint := range results {
		ruleSeverity := strings.ToLower(lint.RuleSeverity)
		level, ok := LintSeverity[ruleSeverity]
		if !ok {
			level = severityLevelOff
		}
		messages[i] = LintMessage{
			RuleID:   lint.RuleName,
			Severity: level,
			Line:     lint.StartPosition.Line + 1,
			Column:   lint.StartPosition.Character + 1,
			Message:  lint.Failure,
		}
	}
	return messages, nil
}

// SCSSLint lints the scss files
func SCSSLint(fileName, cwd string) ([]LintMessage, error) {
	var cmd *exec.Cmd
	cmd = exec.Command(Conf.Core.SCSSLint, "--format=JSON", fileName)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, err
		}
	}

	LogAccess.Debugf("SCSSLint Result:\n%s", out)

	var results map[string][]SCSSLintResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		return nil, err
	}

	if len(results) <= 0 {
		return []LintMessage{}, nil
	}

	var messages []LintMessage
	for _, lints := range results {
		messages = make([]LintMessage, len(lints))
		for i, lint := range lints {
			ruleSeverity := strings.ToLower(lint.Severity)
			level, ok := LintSeverity[ruleSeverity]
			if !ok {
				level = severityLevelOff
			}
			messages[i] = LintMessage{
				RuleID:   lint.Linter,
				Severity: level,
				Line:     lint.Line,
				Column:   lint.Column,
				Message:  lint.Reason,
			}
		}
		break
	}
	return messages, nil
}

// Goreturns formats the go code
func Goreturns(filePath, repoPath string) (lints []LintMessage, err error) {
	ruleID := ruleGoreturns
	fileDiff, err := goreturns(filePath)
	if err != nil {
		return nil, err
	}
	if fileDiff != nil {
		for _, hunk := range fileDiff.Hunks {
			delta := getOrigBeginningDelta(hunk)
			lints = append(lints, LintMessage{
				RuleID:   ruleID,
				Line:     int(hunk.OrigStartLine) + delta,
				Column:   int(hunk.OrigLines) - delta,
				Message:  "\n```diff\n" + string(hunk.Body) + "```",
				Severity: severityLevelError,
			})
		}
	}
	return lints, nil
}

// Golint lints the go file
func Golint(filePath, repoPath string) (lints []LintMessage, err error) {
	ruleID := ruleGolint
	ps, err := golint(filePath)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if p.Confidence >= golintMinConfidenceDefault {
			lints = append(lints, LintMessage{
				RuleID:   ruleID,
				Severity: severityLevelError,
				Line:     p.Position.Line,
				Column:   p.Position.Column,
				Message:  p.Text,
			})
		}
	}
	return lints, nil
}

func goreturns(filePath string) (*diff.FileDiff, error) {
	pkgDir := filepath.Dir(filePath)

	opt := &returns.Options{}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	// src holds the original content.
	var res = src

	res, err = imports.Process(filePath, res, &imports.Options{
		Fragment:  opt.Fragment,
		AllErrors: opt.AllErrors,
		Comments:  true,
		TabIndent: true,
		TabWidth:  8,
	})
	if err != nil {
		return nil, err
	}

	res, err = returns.Process(pkgDir, filePath, res, opt)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(src, res) {
		udf := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(src)),
			B:        difflib.SplitLines(string(res)),
			FromFile: "original",
			ToFile:   "formatted",
			Context:  0,
		}
		data, err := difflib.GetUnifiedDiffString(udf)
		if err != nil {
			return nil, fmt.Errorf("computing diff: %s", err)
		}
		return diff.ParseFileDiff([]byte(data))
	}
	return nil, nil
}

func golint(filePath string) ([]lint.Problem, error) {
	files := make(map[string][]byte)
	src, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	files[filePath] = src

	l := new(lint.Linter)
	ps, err := l.LintFiles(files)
	if err != nil {
		return nil, err
	}
	return ps, nil
}
