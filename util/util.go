package util

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseFloatPercent converts percentages string to float number
func ParseFloatPercent(s string, bitSize int) (f float64, err error) {
	i := strings.Index(s, "%")
	if i < 0 {
		return 0, fmt.Errorf("ParseFloatPercent: percentage sign not found")
	}
	f, err = strconv.ParseFloat(s[:i], bitSize)
	if err != nil {
		return 0, err
	}
	return f / 100, nil
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

// FileExists returns true if filename exists and is not a directory
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// Truncated("1200 0000 0000 0034", " ... ", 9) = "12 ... 34"
func Truncated(s string, t string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(t) > n {
		return Truncated(t, "", n)
	}

	p := n - len(t)

	b := p / 2
	e := p - b
	return s[:b] + t + s[len(s)-e:]
}
