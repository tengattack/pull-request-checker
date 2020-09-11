package util

import (
	"bufio"
	"os"
)

// HeadFile get file's head line
func HeadFile(file string, n int) (lines []string, err error) {
	if n < 1 {
		panic("n should not be less than 1")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	for i := 1; s.Scan() && i <= n; i++ {
		lines = append(lines, s.Text())
	}
	return lines, s.Err()
}

// FileExists returns true if filename exists and is not a directory
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	if info == nil {
		return false
	}
	return !info.IsDir()
}
