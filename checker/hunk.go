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
	delta := len(lines) - 1
	for i := len(lines)-1; i>=0 && delta > 0; i-- {
		if len(lines[i])>0 && lines[i][0] != ' ' {
			break
		}
		delta--
	}
	return delta
}
