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
