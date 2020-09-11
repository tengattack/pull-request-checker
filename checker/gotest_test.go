package checker

import (
	"bytes"
	"context"
	"io"
	"path"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	shellwords "github.com/mattn/go-shellwords"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/util"
	"golang.org/x/sync/errgroup"
)

func TestCoverRegex(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filepath, _, _ := runtime.Caller(0)
	curDir := path.Dir(filepath)
	repo := curDir + "/../testdata/go"

	repoConf, err := util.ReadProjectConfig(repo)
	tests := repoConf.Tests
	require.NoError(err)
	test, ok := tests["go"]
	require.True(ok)

	parser := shellwords.NewParser()
	parser.ParseEnv = true
	parser.ParseBacktick = true
	parser.Dir = repo

	var result string
	var output string
	var pct float64
	log := new(strings.Builder)
	for _, cmd := range test.Cmds {
		out := new(strings.Builder)
		w := io.MultiWriter(log, out)
		errCmd := carry(context.Background(), parser, repo, cmd, w)
		assert.NoError(errCmd)
		output += ("\n" + out.String())
	}

	if test.Coverage != "" {
		result, pct, err = parseCoverage(test.Coverage, output)
		assert.NoError(err)
	}
	assert.Regexp(percentageRegexp, result)
	assert.True(pct > 0)
}

func TestLogDivider(t *testing.T) {
	assert := assert.New(t)

	var b bytes.Buffer
	lg := NewLogDivider(true, &b)
	var eg errgroup.Group
	eg.Go(func() error {
		lg.Log(
			func(w io.Writer) {
				_, _ = w.Write([]byte{byte('1')})
				time.Sleep(1 * time.Millisecond)
				_, _ = w.Write([]byte{byte('2')})
				time.Sleep(1 * time.Millisecond)
				_, _ = w.Write([]byte{byte('3')})
			},
		)
		return nil
	})
	eg.Go(func() error {
		lg.Log(
			func(w io.Writer) {
				_, _ = w.Write([]byte{byte('4')})
				time.Sleep(1 * time.Millisecond)
				_, _ = w.Write([]byte{byte('5')})
				time.Sleep(1 * time.Millisecond)
				_, _ = w.Write([]byte{byte('6')})
			},
		)
		return nil
	})
	eg.Go(func() error {
		lg.Log(
			func(w io.Writer) {
				_, _ = w.Write([]byte{byte('7')})
				time.Sleep(1 * time.Millisecond)
				_, _ = w.Write([]byte{byte('8')})
				time.Sleep(1 * time.Millisecond)
				_, _ = w.Write([]byte{byte('9')})
			},
		)
		return nil
	})
	_ = eg.Wait()

	s := b.String()
	assert.Contains(s, "123")
	assert.Contains(s, "456")
	assert.Contains(s, "789")

	b.Reset()
	lg = NewLogDivider(false, &b)
	c := make(chan int)

	go lg.Log(func(w io.Writer) {
		for i := 1; i <= 2; i++ {
			_, _ = w.Write([]byte(strconv.Itoa(i)))
			_, _ = w.Write([]byte{byte('\n')})
			c <- 0
		}
	})
	for i := 1; i <= 2; i++ {
		<-c
		line, _ := b.ReadString('\n')
		assert.Equal(strconv.Itoa(i)+"\n", line)
	}
}
