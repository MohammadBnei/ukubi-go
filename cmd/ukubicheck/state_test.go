package main

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestPkgState: one case per declaration form, per write form and per table misuse, a cross-package write and a
// cross-package misuse (through the table fact), a hand-written write into a generated variable, and the negatives:
// embeds, made values (connectkeys.New's Keys with its methods called), the blank, table reads, a local, another
// module's variable, generated and test files.
func TestPkgState(t *testing.T) {
	analysistest.Run(t, filepath.Join(analysistest.TestData(), "pkgstate"), pkgState(), "./...")
}
