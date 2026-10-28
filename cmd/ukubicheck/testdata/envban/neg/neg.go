// Package neg reads nothing: os's other names, a Getenv method of its own.
package neg

import (
	"fmt"
	"os"
)

type env map[string]string

func (e env) Getenv(k string) string { return e[k] }

func f() {
	fmt.Fprintln(os.Stderr, os.Args, env{}.Getenv("X"))
}
