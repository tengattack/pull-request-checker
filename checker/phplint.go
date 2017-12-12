package checker

import (
	"encoding/json"
	"os/exec"
)

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

// PHPLint lints the php file
func PHPLint(fileName string) ([]LintMessage, error) {
	var cmd *exec.Cmd
	if len(Conf.Core.PHPLintConfig) > 0 {
		cmd = exec.Command("php", Conf.Core.PHPLint, "-f", "json", "-c", Conf.Core.PHPLintConfig, fileName)
	} else {
		cmd = exec.Command("php", Conf.Core.PHPLint, "-f", "json", fileName)
	}
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
