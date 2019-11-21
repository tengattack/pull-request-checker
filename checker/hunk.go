package checker

import (
	"strings"

	"github.com/sourcegraph/go-diff/diff"
)

func getNumberofContextLines(hunk *diff.Hunk, limit int) int {
	if hunk == nil {
		return 0
	}
	// skip the preceding common context lines
	delta := 0
	lines := strings.Split(string(hunk.Body), "\n")
	for i := 0; i < len(lines) && delta < limit; i++ {
		if len(lines[i]) == 0 {
			continue
		}
		if lines[i][0] != ' ' {
			break
		}
		delta++
	}
	return delta
}

func getOffsetToUnifiedDiff(targetLine int, hunk *diff.Hunk) int {
	if hunk == nil {
		return 0
	}
	lines := strings.Split(string(hunk.Body), "\n")
	if len(lines) <= 1 {
		return 0
	}
	i := len(lines) - 1
	if lines[len(lines)-1] == "" {
		i--
	}
	if targetLine < int(hunk.NewStartLine) || targetLine >= int(hunk.NewStartLine+hunk.NewLines) {
		return i
	}
	currentLine := int(hunk.NewStartLine + hunk.NewLines - 1)
	currentLineOffset := i

	for ; i > 0; i-- {
		if len(lines[i]) == 0 {
			break
		}
		if lines[i][0] == ' ' || lines[i][0] == '+' {
			break
		}
		if lines[i][0] == '-' || lines[i][0] == '\\' {
			currentLineOffset--
		}
	}

	for ; i > 0; i-- {
		if currentLine <= targetLine {
			break
		}
		if len(lines[i]) == 0 {
			currentLine--
			currentLineOffset--
			continue
		}
		if lines[i][0] == ' ' || lines[i][0] == '+' {
			currentLine--
			currentLineOffset--
			continue
		}
		if lines[i][0] == '-' || lines[i][0] == '\\' {
			currentLineOffset--
		}
	}
	return currentLineOffset
}

func getLintsFromDiff(fileDiff *diff.FileDiff, lints []LintMessage, ruleID string) []LintMessage {
	if fileDiff != nil {
		for _, hunk := range fileDiff.Hunks {
			delta := getNumberofContextLines(hunk, int(hunk.OrigLines))
			size := int(hunk.OrigLines) - delta
			if hunk.OrigLines == 0 {
				size = 1
			}
			lints = append(lints, LintMessage{
				RuleID:   ruleID,
				Line:     int(hunk.OrigStartLine) + delta,
				Column:   size,
				Message:  "\n```diff\n" + string(hunk.Body) + "```",
				Severity: severityLevelError,
			})
		}
	}
	return lints
}
