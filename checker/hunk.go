package checker

import (
	"strings"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

func getNumberofContextLines(hunk *diff.Hunk, limit int32) int {
	if hunk == nil {
		return 0
	}
	// skip the preceding common context lines
	delta := 0
	lines := strings.Split(string(hunk.Body), "\n")
	for i := 0; i < len(lines) && delta < int(limit); i++ {
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
	if targetLine < int(hunk.NewStartLine) || targetLine >= int(hunk.NewStartLine+ hunk.NewLines) {
		return 0
	}
	currentLine := int(hunk.NewStartLine)
	currentLineOffset := 0

	lines := strings.Split(string(hunk.Body), "\n")
	i:=0;
	for ; i<len(lines); i++ {
		if len(lines[i]) <= 0 {
			continue
		}
		if lines[i][0] == ' ' || lines[i][0] == '+' {
			break
		}
		if lines[i][0] == '-' || lines[i][0] == '\\'{
			currentLineOffset++
		}
	}

	for ; i<len(lines); i++ {
		if len(lines[i]) <= 0 {
			continue
		}
		if currentLine >= targetLine {
			break
		}
		if lines[i][0] == ' ' || lines[i][0] == '+' {
			currentLine++
			currentLineOffset++
		}
		if lines[i][0] == '-' || lines[i][0] == '\\'{
			currentLineOffset++
		}
	}
	return currentLineOffset
}
