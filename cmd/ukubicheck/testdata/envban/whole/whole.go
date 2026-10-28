// Package whole imports what is refused whole, under any name.
package whole

import (
	_ "golang.org/x/sys/execabs" // want `^import "golang\.org/x/sys/execabs":`
	"os/exec"                    // want `^import "os/exec":`
	u "os/user"                  // want `^import "os/user":`
)

var _, _ = exec.LookPath, u.Current
