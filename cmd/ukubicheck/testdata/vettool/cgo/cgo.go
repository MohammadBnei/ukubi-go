// Package cgo imports "C": with cgo on, vet hands over cmd/cgo's rewrite; with it off, the file is excluded.
package cgo

import "C"

var cgoState int // cmd/cgo's rewrite is generated, but the file is the user's: checked with cgo on

func Two() int { return cgoState }
