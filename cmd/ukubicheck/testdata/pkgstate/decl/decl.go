// Package decl declares package-level variables, writes them, and reads its tables.
package decl

import (
	"embed"
	"errors"
	"maps"
	"regexp"
	"slices"

	"github.com/MohammadBnei/ukubi-go/errs/connectkeys"
)

var plain int // want `^plain is package-level state`

var (
	grouped    int                                       // want `^grouped is package-level state`
	Exported   = 2                                       // want `^Exported is package-level state`
	st         struct{ f int }                           // want `^st is package-level state`
	arr        [2]int                                    // want `^arr is package-level state`
	sl         []int                                     // want `^sl is package-level state`
	m          = map[string]int{}                        // want `^m is package-level state`
	reMade, re = regexp.MustCompile("a"), regexp.Compile // want `^re is package-level state`
)

//go:embed x.txt
var raw []byte // want `^raw is package-level state`

//go:embed x.txt
var text string

//go:embed x.txt
var files embed.FS

var _ = 1

var errX = errors.New("x")

var keys = connectkeys.New()

func W() any { return keys.WithInterceptors() }

func init() {} // want `^func init\(\) wires package-level state`

//ukubicheck:table the lookups
var tbl = map[string][]int{"a": {1}} // want tbl:"table"

//ukubicheck:table
var noReason = []int{1} // want `^noReason: //ukubicheck:table takes a reason` noReason:"table"

// Tables, a group of them.
//
//ukubicheck:table the ints and the records
var (
	Lst  = []int{3, 1} // want Lst:"table"
	ints = [2]int{1}   // want ints:"table"
	recs = []rec{{}}   // want recs:"table"
	tt   = T{1}        // want tt:"table"
)

type rec struct {
	f []int
	n int
}

type T []int

func (T) M() {}

func writes() {
	plain = 1              // want `^assigns decl\.plain: package-level state is never written`
	plain += 1             // want `^assigns decl\.plain`
	for plain = range sl { // want `^assigns \(a range's\) decl\.plain`
	}
	plain++            // want `^steps decl\.plain`
	_ = &plain         // want `^takes the address of decl\.plain`
	delete(m, "k")     // want `^deletes into decl\.m`
	clear(sl)          // want `^clears into decl\.sl`
	copy(sl, []int{1}) // want `^copys into decl\.sl`
	st.f = 1           // want `^assigns decl\.st`
	arr[0] = 1         // want `^assigns decl\.arr`
	sl[0:1][0] = 1     // want `^assigns decl\.sl`
	(*(&arr))[1] = 2   // want `^assigns decl\.arr` `^takes the address of decl\.arr`
	x := 1
	x = 2
	_ = x
}

func misuse() []int {
	slices.Sort(Lst)       // want `^Lst is a table: it is only read`
	slices.Sort(tbl["a"])  // want `^tbl is a table`
	slices.Sort(recs[0].f) // want `^recs is a table`
	_ = append(Lst, 1)     // want `^Lst is a table`
	alias := Lst           // want `^Lst is a table`
	el := tbl["a"]         // want `^tbl is a table`
	_ = tt.M               // want `^tt is a table`
	_, _ = alias, el
	return Lst // want `^Lst is a table`
}

func reads() int {
	n := Lst[0] + ints[1] + recs[0].n + len(Lst) + cap(Lst) + len(tbl["a"])
	for range Lst {
	}
	for _, v := range tbl["a"] {
		n += v
	}
	_, _ = slices.Contains(Lst, 1), slices.Index(Lst, 1)
	_, _ = slices.Clone(Lst), maps.Clone(tbl)
	_ = tbl == nil
	r := recs[0]
	return n + r.n + len(noReason) + len(text) + len(raw) + plain + grouped + Exported + st.f + arr[0] + len(m)
}

var _, _, _, _, _ = files, errX, reMade, re, sl
