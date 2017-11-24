package checker

import (
	"encoding/json"
	"os/exec"
)

// Lint is a single lint result for PHPLint
type Lint struct {
	RuleID     string `json:"ruleId"`
	Severity   int    `json:"severity"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Message    string `json:"message"`
	SourceCode string `json:"sourceCode"`
}

// PHPLint lints the php file
func PHPLint(fileName string) ([]Lint, error) {
	cmd := exec.Command("php", Conf.Core.PHPLint, fileName)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	LogAccess.Debugf("PHPLint Result:\n%s", out)

	var lints []Lint
	err = json.Unmarshal(out, &lints)
	if err != nil {
		return nil, err
	}

	return lints, nil
}
