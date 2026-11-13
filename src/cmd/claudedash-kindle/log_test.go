package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogStaysUnderItsLimitWhileRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claudedash.log")
	logf := newLogger(path, 2000, 1000)
	for i := 0; i < 500; i++ {
		logf("line %03d %s", i, strings.Repeat("x", 20))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 2100 {
		t.Fatalf("log is %d bytes", len(data))
	}
	if !strings.Contains(string(data), "line 499") {
		t.Fatal("the newest lines are kept")
	}
}

func TestLogTrimsAnOldFileAtStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claudedash.log")
	os.WriteFile(path, []byte(strings.Repeat("old line\n", 1000)), 0o644)
	newLogger(path, 2000, 1000)("new")
	data, _ := os.ReadFile(path)
	if len(data) > 1100 || !strings.HasSuffix(strings.TrimSpace(string(data)), "new") {
		t.Fatalf("%d bytes", len(data))
	}
}
