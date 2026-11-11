package applog

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEntriesNewestFirst(t *testing.T) {
	l := New(nil, 10)
	l.Add(Usage, "first")
	l.Add(Kindle, "second %d", 2)
	got := l.Entries()
	if len(got) != 2 || got[0].Message != "second 2" || got[0].Category != Kindle || got[1].Message != "first" {
		t.Fatalf("got %+v", got)
	}
}

func TestOldestDroppedPastCapacity(t *testing.T) {
	l := New(nil, 3)
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		l.Add(System, "%s", m)
	}
	got := l.Entries()
	if len(got) != 3 || got[0].Message != "e" || got[2].Message != "c" {
		t.Fatalf("got %+v", got)
	}
}

func TestEntriesIsACopy(t *testing.T) {
	l := New(nil, 3)
	l.Add(System, "kept")
	l.Entries()[0].Message = "changed"
	if l.Entries()[0].Message != "kept" {
		t.Fatal("callers must not be able to edit the log")
	}
}

func TestWritesToFileWithTimeAndCategory(t *testing.T) {
	var file bytes.Buffer
	l := New(&file, 3)
	l.now = func() time.Time { return time.Date(2026, 11, 11, 14, 30, 5, 0, time.Local) }
	l.Add(Token, "refreshed until %s", "18:00")
	if got := file.String(); got != "2026/11/11 14:30:05 token: refreshed until 18:00\n" {
		t.Fatalf("got %q", got)
	}
	if l.Entries()[0].Time != l.now().Unix() {
		t.Fatal("entry time not recorded")
	}
}

func TestFormatVerbsInMessageAreSafe(t *testing.T) {
	l := New(nil, 3)
	l.Add(Media, "%s", "100% volume %d")
	if got := l.Entries()[0].Message; got != "100% volume %d" {
		t.Fatalf("got %q", got)
	}
}

func TestConcurrentUse(t *testing.T) {
	var file bytes.Buffer
	l := New(&file, 50)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				l.Add(Usage, "line")
				l.Entries()
			}
		}()
	}
	wg.Wait()
	if len(l.Entries()) != 50 || strings.Count(file.String(), "\n") != 400 {
		t.Fatalf("entries %d lines %d", len(l.Entries()), strings.Count(file.String(), "\n"))
	}
}
