package util

import (
	"bytes"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
)

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
