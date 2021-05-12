package util

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/tengattack/unified-ci/common"
)

// RunGitCommand run git command with proxy if possible
func RunGitCommand(ref common.GithubRef, dir string, args []string, output io.Writer) error {
	parser := NewShellParser(dir, ref)
	words, err := parser.Parse(common.Conf.Core.GitCommand)
	if err != nil {
		return fmt.Errorf("parse git command error: %v", err)
	}
	gitCmds := make([]string, len(words), len(words)+len(args))
	copy(gitCmds, words)
	gitCmds = append(gitCmds, args...)
	cmd := exec.CommandContext(context.Background(), gitCmds[0], gitCmds[1:]...)
	if output != nil {
		cmd.Stdout = output
		cmd.Stderr = output
	}
	if common.Conf.Core.Socks5Proxy != "" {
		cmd.Env = []string{"all_proxy=" + common.Conf.Core.Socks5Proxy}
	} else if common.Conf.Core.HTTPProxy != "" {
		cmd.Env = []string{
			"http_proxy=" + common.Conf.Core.HTTPProxy,
			"https_proxy=" + common.Conf.Core.HTTPProxy,
		}
	}
	cmd.Dir = dir
	return cmd.Run()
}
