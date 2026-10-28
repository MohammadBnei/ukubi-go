// Package alias reads under other import names.
package alias

import (
	h "net/http"
	o "os"
)

func f() {
	o.Getenv("X")              // want `^os\.Getenv:`
	_ = h.ProxyFromEnvironment // want `^net/http\.ProxyFromEnvironment:`
}
