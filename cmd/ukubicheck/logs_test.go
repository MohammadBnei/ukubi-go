package main

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestLogLevel: each way to reach the log package (a call, an alias, a dot import, a method value, a *log.Logger
// method, a constant), the negatives (the Logger type, slog, a wrapper package, a test file) and the positive
// control: a wrapper that uses no log.
func TestLogLevel(t *testing.T) {
	a := logLevel()
	if err := a.Flags.Set("wrappers", "wrap,quiet"); err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, filepath.Join(analysistest.TestData(), "loglevel"), a, "./...")
}
