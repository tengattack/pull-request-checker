package util

import (
	"errors"
	"strconv"
	"strings"

	"github.com/sourcegraph/go-diff/diff"
)

// ParseFileModeInDiff get file mode in diff
func ParseFileModeInDiff(extended []string) (int, error) {
	for _, v := range extended {
		if strings.HasPrefix(v, "index") {
			subs := strings.Split(v, " ")
			if len(subs) > 2 {
				if len(subs[2]) > 3 {
					mode, err := strconv.ParseInt(subs[2][len(subs[2])-3:], 8, 32)
					return int(mode), err
				}
				return 0, errors.New("Unknown extended lines in git diff")
			}
		} else if strings.HasPrefix(v, "new file mode") {
			mode, err := strconv.ParseInt(v[len(v)-3:], 8, 32)
			return int(mode), err
		}
	}
	return 0, nil
}

// GetTrimmedNewName get new file's trimmed name in diff
func GetTrimmedNewName(d *diff.FileDiff) (string, bool) {
	newName := Unquote(d.NewName)
	if strings.HasPrefix(newName, "b/") {
		return newName[2:], true
	}
	return newName, false
}

// GetNumberOfContextLines get the number of context lines
func GetNumberOfContextLines(hunk *diff.Hunk, limit int) int {
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
