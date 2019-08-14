package util

import (
	"fmt"
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
