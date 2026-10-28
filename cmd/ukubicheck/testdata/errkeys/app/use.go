package app

import (
	"fmt"

	"github.com/MohammadBnei/ukubi-go/errs"
)

func use(errs.Key) {}

func made(n int) {
	use("app.lit")                       // want `^a literal makes a key: a key is one of the registry's constants`
	use(raw)                             // want `^the untyped constant raw makes a key`
	use(errs.Key("app.x"))               // want `^a conversion makes a key`
	use(errs.Key(fmt.Sprint("app.", n))) // want `^a conversion makes a key`
	use(KeyA + KeyB)                     // want `^an operator makes a key`
	if k := get(); k == "app.cmp" {      // want `^a literal makes a key`
		use(k)
	}
}

const local errs.Key = "app.local" // want `^a literal makes a key`

type holder struct{ k errs.Key }

func get() errs.Key { return KeyA }

func passed(k errs.Key, h holder, m []errs.Key) {
	use(KeyA)
	use(errs.KeyInternal)
	use(local)
	use(k)
	use(h.k)
	use(get())
	use(m[0])
	use(conv)
	use((KeyB))
}
