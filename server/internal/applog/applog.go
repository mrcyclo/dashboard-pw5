// Package applog keeps the most recent log lines in memory for the window's
// log page, and writes every line to the log file as well.
package applog

import (
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	Usage  = "usage"
	Token  = "token"
	Kindle = "kindle"
	Media  = "media"
	System = "system"
)

type Entry struct {
	Time     int64  `json:"time"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

type Log struct {
	file     io.Writer
	capacity int
	now      func() time.Time

	mu      sync.Mutex
	entries []Entry
}

// New keeps up to capacity entries. file may be nil.
func New(file io.Writer, capacity int) *Log {
	return &Log{file: file, capacity: max(capacity, 1), now: time.Now}
}

func (l *Log) Add(category, format string, args ...any) {
	entry := Entry{Time: l.now().Unix(), Category: category, Message: fmt.Sprintf(format, args...)}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.capacity {
		l.entries = l.entries[len(l.entries)-l.capacity:]
	}
	if l.file == nil {
		return
	}
	stamp := time.Unix(entry.Time, 0).Format("2006/01/02 15:04:05")
	fmt.Fprintf(l.file, "%s %s: %s\n", stamp, entry.Category, entry.Message)
}

// Entries returns a copy, newest first.
func (l *Log) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, len(l.entries))
	for i, entry := range l.entries {
		out[len(l.entries)-1-i] = entry
	}
	return out
}
