// Package other writes another package's state and misuses its table: through the fact.
package other

import (
	"flag"
	"slices"

	"example.com/pkgstate/decl"
	"example.com/pkgstate/gen"
)

func f() {
	decl.Exported = 1     // want `^assigns decl\.Exported`
	gen.G = 2             // want `^assigns gen\.G`
	slices.Sort(decl.Lst) // want `^Lst is a table`
	_ = decl.Lst[0]
	flag.Usage = nil // another module's state: not this module's to hold
}
