package checker

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// LintEnabled list enabled linter
type LintEnabled struct {
	PHP        bool
	TypeScript bool
	SCSS       bool
}

// LintMessage is a single lint message for PHPLint
type LintMessage struct {
	RuleID     string `json:"ruleId"`
	Severity   int    `json:"severity"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Message    string `json:"message"`
	SourceCode string `json:"sourceCode"`
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
		"off":     0,
		"warning": 1,
		"error":   2,
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

// TSLint lints the ts and tsx files
func TSLint(fileName, cwd string) ([]LintMessage, error) {
	var cmd *exec.Cmd
	cmd = exec.Command("node", Conf.Core.TSLint, "--format", "json", fileName)
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
			level = 0
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
				level = 0
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
