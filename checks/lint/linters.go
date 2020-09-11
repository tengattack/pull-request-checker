package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/martinlindhe/go-difflib/difflib"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/sqs/goreturns/returns"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/util"
	"golang.org/x/lint"
	"golang.org/x/tools/imports"
)

// A value in (0,1] estimating the confidence of correctness in golint reports
// This value is used internally by golint. Its default value is 0.8
const golintMinConfidenceDefault = 0.8
const (
	SeverityLevelOff = iota
	SeverityLevelWarning
	SeverityLevelError
)
const (
	ruleGolint            = "golint"
	ruleGoreturns         = "goreturns"
	ruleMarkdownFormatted = "remark"
	ruleClangLint         = "clanglint"
)

// LintEnabled list enabled linter
type LintEnabled struct {
	CPP        bool
	OC         bool
	ClangLint  bool
	Go         bool
	PHP        bool
	TypeScript bool
	SCSS       bool
	JS         string
	ES         string
	MD         bool
	APIDoc     bool
	Android    bool
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
		"off":     SeverityLevelOff,
		"warning": SeverityLevelWarning,
		"error":   SeverityLevelError,
	}
}

func isCPP(fileName string) bool {
	ext := []string{".c", ".cc", ".h", ".hpp", ".c++", ".h++", ".cu", ".cpp", ".hxx", ".cxx", ".cuh"}
	for i := 0; i < len(ext); i++ {
		if strings.HasSuffix(fileName, ext[i]) {
			return true
		}
	}
	return false
}

func isOC(fileName string) bool {
	i := strings.LastIndex(fileName, ".")
	if i == -1 {
		return false
	}
	ext := fileName[i:]
	switch ext {
	case ".c", ".cc", ".cpp", ".h", ".m", ".mm":
		return true
	default:
		return false
	}
}

// Init default LintEnabled struct
func (lintEnabled *LintEnabled) Init(cwd string) {

	// reset to defaults
	lintEnabled.CPP = false
	lintEnabled.OC = false
	lintEnabled.ClangLint = false
	lintEnabled.Go = false
	lintEnabled.PHP = true
	lintEnabled.TypeScript = false
	lintEnabled.SCSS = false
	lintEnabled.JS = ""
	lintEnabled.ES = ""
	lintEnabled.MD = false
	lintEnabled.APIDoc = false
	lintEnabled.Android = false

	if _, err := os.Stat(filepath.Join(cwd, ".golangci.yml")); err == nil {
		lintEnabled.Go = true
	}
	if _, err := os.Stat(filepath.Join(cwd, "CPPLINT.cfg")); err == nil {
		lintEnabled.CPP = true
	}
	if _, err := os.Stat(filepath.Join(cwd, ".oclint")); err == nil {
		lintEnabled.OC = true
	}
	if _, err := os.Stat(filepath.Join(cwd, ".clang-format")); err == nil {
		lintEnabled.ClangLint = true
	}
	if _, err := os.Stat(filepath.Join(cwd, ".remarkrc")); err == nil {
		lintEnabled.MD = true
	} else if _, err := os.Stat(filepath.Join(cwd, ".remarkrc.js")); err == nil {
		lintEnabled.MD = true
	}
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
	if _, err := os.Stat(filepath.Join(cwd, "apidoc.json")); err == nil {
		lintEnabled.APIDoc = true
	}
	if _, err := os.Stat(filepath.Join(cwd, "build.gradle")); err == nil {
		lintEnabled.Android = true
	}
}

// CPPLint lints the cpp language files using github.com/cpplint/cpplint
func CPPLint(ref common.GithubRef, filePath string, cwd string) (lints []LintMessage, err error) {
	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.CPPLint)
	if err != nil {
		common.LogError.Error("CPPLint: " + err.Error())
		return nil, err
	}
	words = append(words, "--quiet", filePath)
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = cwd

	var output bytes.Buffer
	cmd.Stderr = &output

	// the exit status is not 0 when cpplint finds a problem in code files
	err = cmd.Run()
	if err != nil && err.Error() != "exit status 1" {
		common.LogError.Error("CPPLint: " + err.Error())
		return nil, err
	}
	outputStr := output.String()
	common.LogAccess.Debugf("CPPLint Output:\n%s", outputStr)
	lines := strings.Split(outputStr, "\n")

	// Sample output: "code.cpp:138:  Missing spaces around =  [whitespace/operators] [4]"
	re := regexp.MustCompile(`:(\d+):(.+)\[(.+?)\] \[\d\]\s*$`)
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
				Severity: SeverityLevelError,
				Line:     lineNum,
				Column:   0,
				Message:  msg,
			})
		}
	}
	return lints, nil
}

// OCLintResultXML is the result for OCLint
type OCLintResultXML struct {
	XMLName xml.Name `xml:"oclint"`

	Violations oclintViolations `xml:"violations"`
}

type oclintViolations struct {
	XMLName xml.Name `xml:"violations"`

	Violations []oclintViolation `xml:"violation"`
}

type oclintViolation struct {
	XMLName xml.Name `xml:"violation"`

	Message   string `xml:"message,attr"`
	Rule      string `xml:"rule,attr"`
	StartLine int    `xml:"startline,attr"`
	EndLine   int    `xml:"endline,attr"`
	Path      string `xml:"path,attr"`
}

// OCLint lints objective-c files
func OCLint(ctx context.Context, ref common.GithubRef, filePath string, cwd string) (lints []LintMessage, err error) {
	parser := util.NewShellParser(cwd, ref)
	words, _ := parser.Parse(common.Conf.Core.OCLint)
	if len(words) < 1 {
		return nil, errors.New("Invalid `oclint` configuration")
	}
	words = append(words, "-i", filePath, "--", "-report-type", "xml")

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var stderr bytes.Buffer
	// The provided context is used to kill the process (by calling os.Process.Kill)
	cmd := exec.CommandContext(ctx, words[0], words[1:]...)
	cmd.Stderr = &stderr
	cmd.Dir = cwd
	out, _ := cmd.Output()

	common.LogAccess.Debugf("OCLint Output:\n%s", out)
	common.LogAccess.Debugf("OCLint Stderr:\n%s", stderr.String())

	if len(out) <= 0 {
		// empty result
		return lints, nil
	}

	// parse xml
	var violations OCLintResultXML
	err = xml.Unmarshal(out, &violations)
	if err != nil {
		msg := fmt.Sprintf("OCLint can not parse xml: %v", err)
		common.LogError.Error(msg)
		return nil, errors.New(msg)
	}

	for _, v := range violations.Violations.Violations {
		lints = append(lints, LintMessage{
			RuleID:  v.Rule,
			Line:    v.StartLine,
			Column:  v.EndLine, // %d:%d, using the second number as the endline number in oclint
			Message: v.Message,
		})
	}
	return lints, nil
}

// KtlintJSONReport is used for capturing the json output of ktlint
type KtlintJSONReport struct {
	File   string `json:"file"`
	Errors []struct {
		Line    int    `json:"line"`
		Column  int    `json:"column"`
		Message string `json:"message"`
		Rule    string `json:"rule"`
	} `json:"errors"`
}

// Ktlint runs ktlint configuration
func Ktlint(ctx context.Context, ref common.GithubRef, filepath, cwd string) ([]LintMessage, error) {
	parser := util.NewShellParser(cwd, ref)
	words, _ := parser.Parse(common.Conf.Core.Ktlint)
	if len(words) < 1 {
		return nil, errors.New("Invalid `ktlint` configuration")
	}
	words = append(words, "-a", "--relative", "--reporter=json", filepath)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// The provided context is used to kill the process (by calling os.Process.Kill)
	cmd := exec.CommandContext(ctx, words[0], words[1:]...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if ee.ExitCode() == 1 {
				err = nil
			}
		}
	}
	if err != nil {
		common.LogError.Errorf("Ktlint: %v\n%s", err, out)
	}
	var reports []KtlintJSONReport
	err = json.Unmarshal(out, &reports)
	if err != nil {
		return nil, err
	}
	results := make([]LintMessage, 0, len(reports))
	for _, f := range reports {
		for _, v := range f.Errors {
			results = append(results, LintMessage{
				RuleID:   v.Rule,
				Severity: SeverityLevelWarning,
				Line:     v.Line,
				Column:   v.Column,
				Message:  v.Message,
			})
		}
	}
	return results, err
}

// PHPLint lints the php files
func PHPLint(ref common.GithubRef, fileName, cwd string) ([]LintMessage, string, error) {
	var stderr bytes.Buffer

	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.PHPLint)
	if err != nil {
		common.LogError.Error("PHPLint: " + err.Error())
		return nil, stderr.String(), err
	}
	words = append(words, "-f", "json", fileName)
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Stderr = &stderr
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, stderr.String(), err
	}

	common.LogAccess.Debugf("PHPLint Output:\n%s", out)

	var results []LintResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		return nil, stderr.String(), err
	}

	if len(results) <= 0 {
		return []LintMessage{}, stderr.String(), nil
	}
	return results[0].Messages, stderr.String(), nil
}

// ESLint lints the js, jsx, es, esx files
func ESLint(ref common.GithubRef, fileName, cwd, eslintrc string) ([]LintMessage, string, error) {
	var stderr bytes.Buffer

	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.ESLint)
	if err != nil {
		common.LogError.Error("ESLint: " + err.Error())
		return nil, stderr.String(), err
	}
	if eslintrc != "" {
		words = append(words, "-c", eslintrc, "-f", "json", fileName)
	} else {
		words = append(words, "-f", "json", fileName)
	}
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = cwd
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, stderr.String(), err
		}
	}

	common.LogAccess.Debugf("ESLint Output:\n%s", out)

	var results []LintResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		return nil, stderr.String(), err
	}

	if len(results) <= 0 {
		return []LintMessage{}, stderr.String(), nil
	}
	return results[0].Messages, stderr.String(), nil
}

// TSLint lints the ts and tsx files
func TSLint(ref common.GithubRef, fileName, cwd string) ([]LintMessage, string, error) {
	var stderr bytes.Buffer

	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.TSLint)
	if err != nil {
		common.LogError.Error("TSLint: " + err.Error())
		return nil, stderr.String(), err
	}
	words = append(words, "--format", "json", fileName)
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = cwd
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, stderr.String(), err
		}
	}

	common.LogAccess.Debugf("TSLint Output:\n%s", out)

	var results []TSLintResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		return nil, stderr.String(), err
	}

	if len(results) <= 0 {
		return []LintMessage{}, stderr.String(), nil
	}

	var messages []LintMessage
	messages = make([]LintMessage, len(results))
	for i, lint := range results {
		ruleSeverity := strings.ToLower(lint.RuleSeverity)
		level, ok := LintSeverity[ruleSeverity]
		if !ok {
			level = SeverityLevelOff
		}
		messages[i] = LintMessage{
			RuleID:   lint.RuleName,
			Severity: level,
			Line:     lint.StartPosition.Line + 1,
			Column:   lint.StartPosition.Character + 1,
			Message:  lint.Failure,
		}
	}
	return messages, stderr.String(), nil
}

// SCSSLint lints the scss files
func SCSSLint(ref common.GithubRef, fileName, cwd string) ([]LintMessage, string, error) {
	var stderr bytes.Buffer

	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.SCSSLint)
	if err != nil {
		common.LogError.Error("SCSSLint: " + err.Error())
		return nil, stderr.String(), err
	}
	words = append(words, "--format=JSON", fileName)
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = cwd
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, stderr.String(), err
		}
	}

	common.LogAccess.Debugf("SCSSLint Output:\n%s", out)

	var results map[string][]SCSSLintResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		return nil, stderr.String(), err
	}

	if len(results) <= 0 {
		return []LintMessage{}, stderr.String(), nil
	}

	var messages []LintMessage
	for _, lints := range results {
		messages = make([]LintMessage, len(lints))
		for i, lint := range lints {
			ruleSeverity := strings.ToLower(lint.Severity)
			level, ok := LintSeverity[ruleSeverity]
			if !ok {
				level = SeverityLevelOff
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
	return messages, stderr.String(), nil
}

// GolangCILintResult golangci-lint json out format
type GolangCILintResult struct {
	Issues []struct {
		FromLinter string `json:"FromLinter"`
		Text       string `json:"Text"`
		// SourceLines []string
		// Replacement *struct{}
		// LineRange struct{From:, To:}
		Pos struct {
			Filename string `json:"Filename"`
			Offset   int64  `json:"Offset"`
			Line     int    `json:"Line"`
			Column   int    `json:"Column"`
		} `json:"Pos"`
	} `json:"Issues"`
	// Report struct{} `json:"Report"`
}

// GolangCILint runs `golangci-lint run --out-format json`
func GolangCILint(ctx context.Context, ref common.GithubRef, cwd string) (*GolangCILintResult, string, error) {
	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.GolangCILint)
	if err == nil && len(words) < 1 {
		err = errors.New("GolangCILint command is not configured")
	}
	words = append(words, "run", "--out-format", "json")

	if err != nil {
		common.LogError.Error("GolangCILint: " + err.Error())
		return nil, "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, words[0], words[1:]...)
	cmd.Stderr = &stderr
	cmd.Dir = cwd
	out, _ := cmd.Output()

	common.LogAccess.Debugf("GolangCILint Output:\n%s", out)
	common.LogAccess.Debugf("GolangCILint Errput:\n%s", stderr.String())

	var result GolangCILintResult
	err = json.Unmarshal(out, &result)
	if err != nil {
		common.LogError.Error("GolangCILint: " + err.Error())
		return nil, stderr.String(), err
	}
	if runtime.GOOS == "windows" {
		for i, v := range result.Issues {
			result.Issues[i].Pos.Filename = filepath.ToSlash(v.Pos.Filename)
		}
	}
	return &result, stderr.String(), nil
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
				Severity: SeverityLevelError,
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
		if data == "" {
			// TODO: final EOL
			return nil, nil
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

func remark(ref common.GithubRef, fileName string, cwd string) (reports []remarkReport, out []byte, err error) {
	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.RemarkLint)
	if err != nil {
		common.LogError.Error("RemarkLint: " + err.Error())
		return nil, nil, err
	}
	words = append(words, "--quiet", "--report", "json", fileName)
	cmd := exec.Command(words[0], words[1:]...)
	cmd.Dir = cwd
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

	stdoutStr, err := ioutil.ReadAll(stdout)
	common.LogAccess.Debugf("RemarkLint Stdout:\n%s", stdoutStr)
	if err != nil {
		return nil, nil, err
	}
	stderrStr, err := ioutil.ReadAll(stderr)
	common.LogAccess.Debugf("RemarkLint Stderr:\n%s", stderrStr)
	if err != nil {
		return nil, stdoutStr, err
	}
	err = json.Unmarshal(stderrStr, &reports)
	if err != nil {
		return nil, stdoutStr, err
	}
	return reports, stdoutStr, cmd.Wait()
}

func markdownFormatted(filePath string, result []byte) (*diff.FileDiff, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(src, result) {
		udf := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(src)),
			B:        difflib.SplitLines(string(result)),
			FromFile: "original",
			ToFile:   "formatted",
			Context:  0,
		}
		data, err := difflib.GetUnifiedDiffString(udf)
		if err != nil {
			return nil, fmt.Errorf("computing diff: %s", err)
		}
		if data == "" {
			// TODO: final EOL
			return nil, nil
		}
		return diff.ParseFileDiff([]byte(data))
	}
	return nil, nil
}

// MDFormattedLint generates lint messages from diffs of remark
func MDFormattedLint(filePath string, result []byte) (lints []LintMessage, err error) {
	ruleID := ruleMarkdownFormatted
	fileDiff, err := markdownFormatted(filePath, result)
	if err != nil {
		return nil, err
	}
	lints = getLintsFromDiff(fileDiff, lints, ruleID)
	return lints, nil
}

type apiDocJSON struct {
	FileFilters    string `json:"file-filters"`
	ExcludeFilters string `json:"exclude-filters"`
	Input          string `json:"input"`
}

func parseAPIDocCommands(ref common.GithubRef, cwd string) ([]string, error) {
	var args apiDocJSON

	fileName := path.Join(cwd, "apidoc.json")
	if util.FileExists(fileName) {
		config, err := ioutil.ReadFile(fileName)
		if err != nil {
			common.LogError.Errorf("Can not read %s: %v", fileName, err)
		} else {
			err = json.Unmarshal(config, &args)
			if err != nil {
				common.LogError.Errorf("Can not parse json: %s", fileName)
			}
		}
	}

	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.APIDoc)
	if err == nil && len(words) < 1 {
		err = errors.New("APIDoc command is not configured")
	}
	if err != nil {
		common.LogError.Error("APIDoc: " + err.Error())
		return nil, err
	}

	if args.FileFilters != "" {
		words = append(words, "-f", args.FileFilters)
	}
	if args.ExcludeFilters != "" {
		words = append(words, "-e", args.ExcludeFilters)
	}
	if args.Input != "" {
		words = append(words, "-i", args.Input)
	}
	return words, nil
}

// APIDoc generates apidoc
func APIDoc(ctx context.Context, ref common.GithubRef, cwd string) (string, error) {
	words, err := parseAPIDocCommands(ref, cwd)
	if err != nil {
		return "parseAPIDocCommands error\n", err
	}
	cmd := exec.CommandContext(ctx, words[0], words[1:]...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	return string(output) + "\n", err
}

// CheckstyleResult struct represents a list of gradle checkstyle files
type CheckstyleResult struct {
	XMLName xml.Name `xml:"checkstyle"`

	File []CheckstyleFile `xml:"file"`
}

// CheckstyleFile struct represents a list of gradle checkstyle file
type CheckstyleFile struct {
	XMLName xml.Name `xml:"file"`

	Name  string            `xml:"name,attr"`
	Error []CheckstyleError `xml:"error"`
}

// CheckstyleError struct represents a list of gradle checkstyle errors
type CheckstyleError struct {
	XMLName xml.Name `xml:"error"`

	Line     int    `xml:"line,attr"`
	Column   int    `xml:"column,attr"`
	Severity string `xml:"severity,attr"`
	Message  string `xml:"message,attr"`
	Source   string `xml:"source,attr"`
}

// Issues struct represents a list of Android lint issues
type Issues struct {
	XMLName xml.Name `xml:"issues"`

	Issues []Issue `xml:"issue"`
}

// Issue struct represents a Android lint issue
type Issue struct {
	XMLName xml.Name `xml:"issue"`

	ID       string `xml:"id,attr"`
	Severity string `xml:"severity,attr"`
	Message  string `xml:"message,attr"`
	Category string `xml:"category,attr"`

	Location struct {
		File   string `xml:"file,attr"`
		Line   int    `xml:"line,attr"`
		Column int    `xml:"column,attr"`
	} `xml:"location"`
}

// AndroidLint Android (Gradle) Lint, returns either issues or message
func AndroidLint(ctx context.Context, ref common.GithubRef, cwd string) (*Issues, string, error) {
	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.AndroidLint)
	if len(words) < 1 && err == nil {
		err = errors.New("Android lint command is not configured")
	}
	if err != nil {
		common.LogError.Error("Android lint: " + err.Error())
		return nil, "", err
	}
	if runtime.GOOS == "windows" {
		words[0] = path.Join(cwd, words[0])
	}

	basePath, err := filepath.Abs(cwd)
	if err != nil {
		err = fmt.Errorf("Can not get absolute repo path: %v", err)
		common.LogError.Error(err)
		return nil, "", err
	}

	var outputs strings.Builder

	// TODO: merge results
	// checkstyle first
	var checkstyleResult CheckstyleResult
	doCheckstyle := false
	// TODO: configure checkstyle command
	checkstyleWords := make([]string, len(words))
	for i, w := range words {
		if w == "lint" {
			copy(checkstyleWords, words)
			checkstyleWords[i] = "checkstyle"
			doCheckstyle = true
			break
		}
	}
	if doCheckstyle {
		outputs.WriteString("checkstyle:\n")
		cmd := exec.CommandContext(ctx, checkstyleWords[0], checkstyleWords[1:]...)
		cmd.Dir = cwd
		output, err := cmd.CombinedOutput()
		if err != nil {
			outputs.WriteString(err.Error() + "\n")
			outputs.Write(output)
			common.LogError.Errorf("Android lint (checkstyle): %v\n%s", err, output)
			// PASS
		} else {
			outputs.Write(output)
			fileName := path.Join(cwd, "build/reports/checkstyle/checkstyle.xml")
			if !util.FileExists(fileName) {
				msg := fmt.Sprintf("Can not find checkstyle result file: %s\n", fileName)
				common.LogAccess.Warn(msg)
				fileName = path.Join(cwd, "app/build/reports/checkstyle/checkstyle.xml")
			}
			if !util.FileExists(fileName) {
				msg := fmt.Sprintf("Can not find checkstyle result file: %s\n", fileName)
				common.LogAccess.Warn(msg)
				// PASS
			} else {
				xmls, err := ioutil.ReadFile(fileName)
				if err != nil {
					msg := fmt.Sprintf("Can not read %s: %v\n", fileName, err)
					common.LogError.Error(msg)
					// PASS
				} else {
					err = xml.Unmarshal(xmls, &checkstyleResult)
					if err != nil {
						msg := fmt.Sprintf("Can not parse xml: %v\n", err)
						common.LogError.Error(msg)
						// PASS
					} else {
						for i, v := range checkstyleResult.File {
							relativeFile, err := filepath.Rel(basePath, v.Name)
							if err != nil {
								msg := fmt.Sprintf("Can not get relative path: %v\n", err)
								common.LogError.Error(msg)
								// PASS
								continue
							}
							if runtime.GOOS == "windows" {
								relativeFile = filepath.ToSlash(relativeFile)
							}
							checkstyleResult.File[i].Name = relativeFile
							// strip checkstyle error source name
							for j, e := range v.Error {
								pos := strings.Index(e.Source, "checkstyle")
								if pos >= 0 {
									v.Error[j].Source = e.Source[pos:]
								}
							}
						}
					}
				}
			}
		}
		outputs.WriteString("\n")
	}

	outputs.WriteString("lint:\n")
	cmd := exec.CommandContext(ctx, words[0], words[1:]...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputs.WriteString(err.Error() + "\n")
		outputs.Write(output)
		common.LogError.Errorf("Android lint: %v\n%s", err, output)
		return nil, outputs.String(), err
	}
	outputs.Write(output)

	var issues Issues
	fileName := path.Join(cwd, "app/build/reports/lint-results.xml")
	if !util.FileExists(fileName) {
		err = fmt.Errorf("Can not find %s", fileName)
		common.LogError.Error(err)
		outputs.WriteString(err.Error() + "\n")
		return nil, outputs.String(), err
	}
	xmls, err := ioutil.ReadFile(fileName)
	if err != nil {
		err = fmt.Errorf("Can not read %s: %v", fileName, err)
		common.LogError.Error(err)
		outputs.WriteString(err.Error() + "\n")
		return nil, outputs.String(), err
	}
	err = xml.Unmarshal(xmls, &issues)
	if err != nil {
		err = fmt.Errorf("Can not parse xml: %v", err)
		common.LogError.Error(err)
		outputs.WriteString(err.Error() + "\n")
		return nil, outputs.String(), err
	}

	for i, v := range issues.Issues {
		relativeFile, err := filepath.Rel(basePath, v.Location.File)
		if err != nil {
			err = fmt.Errorf("Can not get relative path for %s: %v", v.Location.File, err)
			common.LogError.Error(err)
			outputs.WriteString(err.Error() + "\n")
			return nil, outputs.String(), err
		}
		if runtime.GOOS == "windows" {
			relativeFile = filepath.ToSlash(relativeFile)
		}
		issues.Issues[i].Location.File = relativeFile
	}

	if len(checkstyleResult.File) > 0 {
		for _, v := range checkstyleResult.File {
			for _, e := range v.Error {
				issue := Issue{
					Severity: e.Severity,
					Message:  e.Message,
				}
				parts := strings.Split(e.Source, ".")
				if len(parts) >= 4 {
					// eg.: checkstyle.checks.indentation.IndentationCheck
					issue.ID = parts[3]
					issue.Category = parts[2]
				} else {
					// eg.: SeparatorWrapDot
					issue.ID = e.Source
				}
				issue.Location.File = v.Name
				issue.Location.Line = e.Line
				issue.Location.Column = e.Column
				issues.Issues = append(issues.Issues, issue)
			}
		}
	}
	return &issues, outputs.String(), nil
}

// ClangLint runs the clang-format lint
func ClangLint(ctx context.Context, ref common.GithubRef, cwd string, filePath string) (lints []LintMessage, err error) {
	parser := util.NewShellParser(cwd, ref)
	words, err := parser.Parse(common.Conf.Core.ClangLint)
	if err != nil {
		return nil, err
	}
	words = append(words, filePath)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, words[0], words[1:]...)
	cmd.Dir = cwd

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// compute the unified diff between src and out
	src, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var fileDiff *diff.FileDiff
	if !bytes.Equal(src, out) {
		udf := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(src)),
			B:        difflib.SplitLines(string(out)),
			FromFile: "original",
			ToFile:   "formatted",
			Context:  0,
		}
		data, err := difflib.GetUnifiedDiffString(udf)
		if err != nil {
			return nil, fmt.Errorf("computing diff error: %s", err)
		}
		if data == "" {
			// TODO: final EOL
			return nil, nil
		}
		fileDiff, err = diff.ParseFileDiff([]byte(data))
		if err != nil {
			return nil, fmt.Errorf("parse diff error: %s", err)
		}
	}

	lints = getLintsFromDiff(fileDiff, lints, ruleClangLint)
	return
}
