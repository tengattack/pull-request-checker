package util

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar"
)

// ParseFloatPercent converts percentages string to float number
func ParseFloatPercent(s string, bitSize int) (f float64, norm string, err error) {
	i := strings.Index(s, "%")
	if i >= 0 {
		s = s[:i]
	}

	f, err = strconv.ParseFloat(s, bitSize)
	if err != nil {
		return 0, "", fmt.Errorf("ParseFloatPercent %q: percent sign not found and not a number", s)
	}
	// normalization
	s += "%"

	return f / 100, s, nil
}

// FormatFloatPercent converts f to percentages string
func FormatFloatPercent(f float64) string {
	return strconv.FormatFloat(f*100, 'f', 2, 64) + "%"
}

// Unquote unquotes the input string if it is quoted
func Unquote(input string) string {
	newName, err := strconv.Unquote(input)
	if err != nil {
		newName = input
	}
	return newName
}

// Truncated returns whether the truncation is performed and the result of s:
// e.g. Truncated("1200 0000 0000 0034", " ... ", 9) = (true, "12 ... 34")
func Truncated(s string, t string, n int) (bool, string) {
	if n <= 0 {
		panic("n <= 0")
	}

	if len(s) <= n {
		return false, s
	}

	if len(t) > n {
		return Truncated(t, "", n)
	}

	p := n - len(t)

	b := p / 2
	e := p - b
	return true, s[:b] + t + s[len(s)-e:]
}

// MatchAny checks if path matches any of the given patterns
func MatchAny(patterns []string, path string) bool {
	for _, pattern := range patterns {
		match, _ := doublestar.Match(pattern, path)
		if match {
			return true
		}
	}
	return false
}
