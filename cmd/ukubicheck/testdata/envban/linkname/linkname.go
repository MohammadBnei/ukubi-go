// Package linkname reaches into the runtime.
package linkname

import _ "unsafe"

//go:linkname nanotime runtime.nanotime // want `^go:linkname:`
func nanotime() int64
