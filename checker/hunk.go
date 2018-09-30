package checker

import (
	"strings"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

func getOrigBeginningDelta(hunk *diff.Hunk) int {
	if hunk == nil {
		return 0
	}
	// skip the preceding common context lines
	delta := 0
	lines := strings.Split(string(hunk.Body), "\n")
	for i := 0; i < len(lines) && delta < int(hunk.OrigLines); i++ {
		if len(lines[i]) > 0 && lines[i][0] != ' ' {
			break
		}
		delta++
	}
	return delta
}

func getNewBeginningDelta(hunk *diff.Hunk) int {
	if hunk == nil {
		return 0
	}
	// skip the preceding common context lines
	delta := 0
	lines := strings.Split(string(hunk.Body), "\n")
	for i := 0; i < len(lines) && delta < int(hunk.NewLines); i++ {
		if len(lines[i]) > 0 && lines[i][0] != ' ' {
			break
		}
		delta++
	}
	return delta
}

func getNewEndingDelta(hunk *diff.Hunk) int {
	if hunk == nil {
		return 0
	}
	lines := strings.Split(string(hunk.Body), "\n")
	if len(lines) == 0 {
		return 0
	}
	delta := len(lines) - 1
	for i := len(lines)-1; i>=0 && delta > 0; i-- {
		if len(lines[i])>0 && lines[i][0] != ' ' {
			break
		}
		delta--
	}
	return delta
}

func getOffsetNew(targetLine int, hunk *diff.Hunk) int {
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