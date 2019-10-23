package checker

import (
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

func getLintsFromDiff(fileDiff *diff.FileDiff, lints []LintMessage, ruleID string) []LintMessage {
	if fileDiff != nil {
		for _, hunk := range fileDiff.Hunks {
			size := int(hunk.OrigLines)
			if hunk.OrigLines == 0 {
				size = 1
			}
			lints = append(lints, LintMessage{
				RuleID:   ruleID,
				Line:     int(hunk.OrigStartLine),
				Column:   size,
				Message:  "\n```diff\n" + string(hunk.Body) + "```",
				Severity: severityLevelError,
			})
		}
	}
	return lints
}
