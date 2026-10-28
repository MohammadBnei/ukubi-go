package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MohammadBnei/ukubi-go/errs"
	"golang.org/x/tools/go/analysis/analysistest"
)

// TestErrKeys: one case per way to make a key outside the registry, the ways a key passes on (a constant, a
// parameter, a variable, a field, a call, an index), the registry's own literals, a test file, and the positive
// controls: a registry with no key constant, a registry that is missing. The testdata's errs stub has the real
// import path, so the -errkeys.type default is the one checked.
func TestErrKeys(t *testing.T) {
	dir := filepath.Join(analysistest.TestData(), "errkeys")
	for pkg, registry := range map[string]string{"./app": "app/keys.go", "./noconst": "noconst/keys.go", "./nofile": "nofile/keys.go"} {
		a := errKeys()
		if err := a.Flags.Set("registry", registry); err != nil {
			t.Fatal(err)
		}
		analysistest.Run(t, dir, a, pkg)
	}
}

// TestKeyTypeDefault: the default -errkeys.type is this module's errs.Key.
func TestKeyTypeDefault(t *testing.T) {
	k := reflect.TypeFor[errs.Key]()
	if want, got := k.PkgPath()+"."+k.Name(), errKeys().Flags.Lookup("type").DefValue; got != want {
		t.Errorf("-errkeys.type defaults to %s, want %s", got, want)
	}
}
