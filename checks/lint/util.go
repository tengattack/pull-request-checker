package lint

import (
	"bytes"
	"fmt"

	"github.com/google/go-github/github"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/tengattack/unified-ci/util"
)

func getLintsFromDiff(fileDiff *diff.FileDiff, lints []LintMessage, ruleID string) []LintMessage {
	if fileDiff != nil {
		for _, hunk := range fileDiff.Hunks {
			delta := util.GetNumberOfContextLines(hunk, int(hunk.OrigLines))
			size := int(hunk.OrigLines) - delta
			if hunk.OrigLines == 0 {
				size = 1
			}
			lints = append(lints, LintMessage{
				RuleID:   ruleID,
				Line:     int(hunk.OrigStartLine) + delta,
				Column:   size,
				Message:  "\n```diff\n" + string(hunk.Body) + "```",
				Severity: SeverityLevelError,
			})
		}
	}
	return lints
}

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
