// Package bad breaks one rule per analyzer.
package bad

import (
	"log"
	"os"

	"example.com/vt/keys"
)

var state int

func use(keys.Key) {}

func f() {
	_ = os.Getenv("VT")
	log.Print(state)
	use("vt.lit")
}
