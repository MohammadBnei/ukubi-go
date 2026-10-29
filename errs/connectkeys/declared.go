package connectkeys

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/MohammadBnei/ukubi-go/errs"
	errorsv1 "github.com/MohammadBnei/ukubi-go/proto/gen/go/ukubi/errors/v1"
)

// CheckDeclared holds services' proto-declared keys (ukubi/errors/v1/options.proto) and the app's Go registry to each
// other. Call it from a test: it walks descriptors, which no request path needs.
//
// A service's key enums are the top-level enums of its file with a value carrying (ukubi.errors.v1.key). It reports a
// non-zero value of a key enum with no wire string, a wire string declared twice, a key an RPC's
// (ukubi.errors.v1.keys) names that its service's file does not declare, a declared key the registry lacks, and a
// registry key nothing declares. KeyInternal, which the fallback gives every RPC, counts as declared and need not be
// in the registry. registry is the app's Connect keys: leave out keys only its JSON bodies use. The error joins one
// finding per line; nil when there is none.
func CheckDeclared(registry []errs.Key, services ...protoreflect.ServiceDescriptor) error {
	var found []error
	bad := func(f string, a ...any) { found = append(found, fmt.Errorf("connectkeys: "+f, a...)) }

	type decl struct {
		key  errs.Key
		name protoreflect.FullName
	}
	var all []decl                                            // in declaration order, for stable findings
	byFile := map[string]map[errs.Key]protoreflect.FullName{} // a file's keys, read once per file
	for _, sd := range services {
		f := sd.ParentFile()
		if byFile[f.Path()] != nil {
			continue
		}
		keys := map[errs.Key]protoreflect.FullName{}
		byFile[f.Path()] = keys
		// ponytail: the enum lives in the service's file, at top level; read imports or nested enums if one must not.
		for i := range f.Enums().Len() {
			vals := f.Enums().Get(i).Values()
			keyed := false
			for j := range vals.Len() {
				keyed = keyed || wire(vals.Get(j)) != ""
			}
			if !keyed {
				continue
			}
			for j := range vals.Len() {
				v := vals.Get(j)
				k := wire(v)
				if k == "" {
					if v.Number() != 0 {
						bad("%s declares no key: give it (ukubi.errors.v1.key)", v.FullName())
					}
					continue
				}
				for _, d := range all {
					if d.key == k {
						bad("%s and %s both declare %q", d.name, v.FullName(), k)
					}
				}
				keys[k] = v.FullName()
				all = append(all, decl{k, v.FullName()})
			}
		}
	}

	for _, sd := range services {
		keys := byFile[sd.ParentFile().Path()]
		for i := range sd.Methods().Len() {
			m := sd.Methods().Get(i)
			for _, k := range proto.GetExtension(m.Options(), errorsv1.E_Keys).([]string) {
				if _, ok := keys[errs.Key(k)]; !ok && errs.Key(k) != KeyInternal {
					bad("%s lists %q, which no key enum in %s declares", m.FullName(), k, sd.ParentFile().Path())
				}
			}
		}
	}

	reg := map[errs.Key]bool{}
	for _, k := range registry {
		reg[k] = true
		declared := k == KeyInternal
		for _, d := range all {
			declared = declared || d.key == k
		}
		if !declared {
			bad("registry key %q is declared by no key enum", k)
		}
	}
	for _, d := range all {
		if !reg[d.key] {
			bad("%s declares %q, which the registry lacks", d.name, d.key)
		}
	}
	return errors.Join(found...)
}

// wire is v's (ukubi.errors.v1.key), "" without one.
func wire(v protoreflect.EnumValueDescriptor) errs.Key {
	return errs.Key(proto.GetExtension(v.Options(), errorsv1.E_Key).(string))
}
