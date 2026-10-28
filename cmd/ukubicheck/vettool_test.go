package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestVettool drives the real path: the built binary under go vet -vettool, its flags through unitchecker, offline,
// with a fresh GOCACHE (go vet's cache key leaves out the files the build excludes, so a warm cache replays a stale
// result for them). What analysistest cannot hold is held here: a file the build excludes (tags) is checked, an
// excluded assembly file is skipped, and cgo is refused whether vet hands over cmd/cgo's rewrite (CGO_ENABLED=1) or
// excludes the file (CGO_ENABLED=0), and cmd/cgo's rewrite of a user file is checked as the user's, not as generated.
// An excluded test file is skipped. A clean package exits 0.
func TestVettool(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "ukubicheck")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	cache := t.TempDir()
	vet := func(cgo, pattern string) ([]string, int) {
		cmd := exec.Command("go", "vet", "-vettool="+bin, "-envban.allow=cfg/config.go", "-errkeys.type=example.com/vt/keys.Key",
			"-errkeys.registry=keys/keys.go", pattern)
		cmd.Dir = filepath.Join("testdata", "vettool")
		cmd.Env = append(os.Environ(), "GOCACHE="+cache, "GOFLAGS=", "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local", "CGO_ENABLED="+cgo)
		out, err := cmd.CombinedOutput()
		code := 0
		if ee := (*exec.ExitError)(nil); errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, l := range strings.Split(string(out), "\n") {
			if diag.MatchString(l) {
				lines = append(lines, l)
			} else if l != "" && !strings.HasPrefix(l, "# ") {
				t.Errorf("CGO_ENABLED=%s %s: unexpected output %q", cgo, pattern, l)
			}
		}
		slices.Sort(lines)
		return lines, code
	}
	want := []string{
		"bad/bad.go:11:5: state is package-level state: it belongs to a value main builds (or, data nothing writes, mark it //ukubicheck:table <reason>)",
		"bad/bad.go:16:9: os.Getenv: only the config file (-envban.allow) reads the environment",
		"bad/bad.go:17:6: log.Print: give the line its level with slog (the log package is for -loglevel.wrappers)",
		"bad/bad.go:18:6: a literal makes a key: a key is one of the registry's constants (-errkeys.registry)",
		"bad/tagged.go:7:31: os.Getenv: only the config file (-envban.allow) reads the environment",
		"cgo/cgo.go:2:1: cgo: only the config file (-envban.allow) reads the environment",
	}
	for _, cgo := range []string{"1", "0"} {
		want := want
		if cgo == "1" {
			want = append(slices.Clone(want), "cgo/cgo.go:6:5: cgoState is package-level state: it belongs to a value main builds (or, data nothing writes, mark it //ukubicheck:table <reason>)")
			slices.Sort(want)
		}
		got, code := vet(cgo, "./...")
		if code != 1 || !slices.Equal(got, want) {
			t.Errorf("CGO_ENABLED=%s ./...: exit %d, want 1; got\n%s\nwant\n%s", cgo, code, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	if got, code := vet("1", "./clean"); code != 0 || len(got) != 0 {
		t.Errorf("./clean: exit %d, want 0; got %q", code, got)
	}
}

var diag = regexp.MustCompile(`^[\w/.-]+\.go:\d+:\d+: `)
