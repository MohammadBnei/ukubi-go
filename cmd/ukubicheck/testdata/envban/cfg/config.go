// Package cfg holds an allowed file: it is allowed whole.
package cfg

import (
	. "os"
	"os/exec"
)

func load() (string, any) { return Getenv("X"), exec.Command }
