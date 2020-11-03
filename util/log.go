package util

import (
	"bytes"
	"io"
	"sync"
)

// LogDivider provides the method to log stuff in parallel
type LogDivider struct {
	buffered bool
	log      io.Writer
	lm       *sync.Mutex
}

// NewLogDivider returns a new LogDivider
func NewLogDivider(buffered bool, log io.Writer) *LogDivider {
	lg := &LogDivider{
		buffered: buffered,
		log:      log,
	}
	if buffered {
		lg.lm = new(sync.Mutex)
	}
	return lg
}

// Log logs the given function f using LogDivider lg
func (lg *LogDivider) Log(f func(io.Writer)) {
	var w io.Writer
	if lg.buffered {
		w = new(bytes.Buffer)
	} else {
		w = lg.log
	}

	f(w)

	if lg.buffered {
		lg.lm.Lock()
		defer lg.lm.Unlock()
		lg.log.Write(w.(*bytes.Buffer).Bytes())
	}
}
