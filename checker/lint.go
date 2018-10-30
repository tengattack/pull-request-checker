package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/martinlindhe/go-difflib/difflib"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/sqs/goreturns/returns"
	"golang.org/x/lint"
	"golang.org/x/tools/imports"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

// A value in (0,1] estimating the confidence of correctness in golint reports
// This value is used internally by golint. Its default value is 0.8
const golintMinConfidenceDefault = 0.8
const (
	severityLevelOff = iota
	severityLevelWarning
	severityLevelError
)
const (
	ruleGolint            = "golint"
	ruleGoreturns         = "goreturns"
	ruleMarkdownFormatted = "remark"
)

// LintEnabled list enabled linter
type LintEnabled struct {
	CPP        bool
	Go         bool
	PHP        bool
	TypeScript bool
	SCSS       bool
	JS         string
	ES         string
	MD         bool
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
	lintEnabled.CPP = true
	lintEnabled.Go = true
	lintEnabled.PHP = true
	lintEnabled.TypeScript = false
	lintEnabled.SCSS = false
	lintEnabled.JS = ""
	lintEnabled.ES = ""
	lintEnabled.MD = true

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

// CPPLint lints the cpp language files using github.com/cpplint/cpplint
func CPPLint(filePath string, repoPath string) (lints []LintMessage, err error) {
	words, err := shellwords.Parse(Conf.Core.CPPLint)
	if err != nil {
		LogError.Error("CPPLint: " + err.Error())
		return nil, err
	}
	words = append(words, "--quiet", filePath)
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = repoPath

	var output bytes.Buffer
	cmd.Stderr = &output

	// the exit status is not 0 when cpplint finds a problem in code files
	err = cmd.Run()
	if err != nil && err.Error() != "exit status 1" {
		LogError.Error("CPPLint: " + err.Error())
		return nil, err
	}
	lines := strings.Split(output.String(), "\n")

	// Sample output: "code.cpp:138:  Missing spaces around =  [whitespace/operators] [4]"
	re := regexp.MustCompile(`:(\d+):(.+)\[(.+?)\] \[\d\]$`)
	for _, line := range lines {
		matched := false
		lineNum := 0
		msg := ""
		rule := ""

		match := re.FindStringSubmatch(line)
		for i, m := range match {
			switch i {
			case 1:
				// line number
				lineNum, _ = strconv.Atoi(m)
			case 2:
				// warning message
				msg = m
			case 3:
				rule = m
				matched = true
			}
		}
		if matched {
			lints = append(lints, LintMessage{
				RuleID:   rule,
				Severity: severityLevelError,
				Line:     lineNum,
				Column:   0,
				Message:  msg,
			})
		}
	}
	return lints, nil
}

// PHPLint lints the php files
func PHPLint(fileName, cwd string) ([]LintMessage, error) {
	words, err := shellwords.Parse(Conf.Core.PHPLint)
	if err != nil {
		LogError.Error("PHPLint: " + err.Error())
		return nil, err
	}
	words = append(words, "-f", "json", fileName)
	cmd := exec.Command(words[0], words[1:]...)
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
	words, err := shellwords.Parse(Conf.Core.ESLint)
	if err != nil {
		LogError.Error("ESLint: " + err.Error())
		return nil, err
	}
	if eslintrc != "" {
		words = append(words, "-c", eslintrc, "-f", "json", fileName)
	} else {
		words = append(words, "-f", "json", fileName)
	}
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, err
		}
	}

	LogAccess.Debugf("ESLint Result:\n%s", out)

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
	words, err := shellwords.Parse(Conf.Core.TSLint)
	if err != nil {
		LogError.Error("TSLint: " + err.Error())
		return nil, err
	}
	words = append(words, "--format", "json", fileName)
	cmd := exec.Command(words[0], words[1:]...)
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
	words, err := shellwords.Parse(Conf.Core.SCSSLint)
	if err != nil {
		LogError.Error("SCSSLint: " + err.Error())
		return nil, err
	}
	words = append(words, "--format=JSON", fileName)
	cmd := exec.Command(words[0], words[1:]...)
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
	lints = getLintsFromDiff(fileDiff, lints, ruleID)
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

// MDLint generates lint messages from the report of remark-lint
func MDLint(rps []remarkReport) (lints []LintMessage, err error) {
	for i, r := range rps {
		if i == 0 {
			for _, m := range r.Messages {
				lints = append(lints, LintMessage{
					RuleID:  m.RuleID,
					Line:    m.Line,
					Message: m.Reason,
				})
			}
		}
	}
	return lints, nil
}

type remarkReport struct {
	Messages []remarkMessage
}

type remarkMessage struct {
	Line   int
	Reason string
	RuleID string
}

func remark(fileName string, repoPath string) (reports []remarkReport, out []byte, err error) {
	cmd := exec.Command("remark", "--quiet", "--report", "json", fileName)
	cmd.Dir = repoPath
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, nil, err
	}

	out, err = ioutil.ReadAll(stdout)
	if err != nil {
		return nil, nil, err
	}
	err = json.NewDecoder(stderr).Decode(&reports)
	if err != nil {
		return nil, out, err
	}
	return reports, out, cmd.Wait()
}

func markdownFormatted(filePath string, res []byte) (*diff.FileDiff, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src, err := ioutil.ReadAll(f)
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

// MDFormattedLint generates lint messages from diffs of remark
func MDFormattedLint(filePath string, res []byte) (lints []LintMessage, err error) {
	ruleID := ruleMarkdownFormatted
	fileDiff, err := markdownFormatted(filePath, res)
	if err != nil {
		return nil, err
	}
	lints = getLintsFromDiff(fileDiff, lints, ruleID)
	return lints, nil
}
