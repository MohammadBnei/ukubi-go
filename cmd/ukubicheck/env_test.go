package main

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestEnvBan: one case per reader and per refused form, the negatives (os's other names, a Getenv method, a test file,
// the allowed file's own reads), and the positive controls: an allowed file that reads nothing, one that is missing.
// Files the build excludes are TestVettool's: analysistest takes no want in them.
func TestEnvBan(t *testing.T) {
	a := envBan()
	if err := a.Flags.Set("allow", "cfg/config.go, empty/config.go,missing/config.go"); err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, filepath.Join(analysistest.TestData(), "envban"), a, "./...")
}
