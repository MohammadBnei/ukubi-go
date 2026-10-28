// Package noconst's registry declares no key: the positive control.
package noconst // want `^-errkeys\.registry noconst/keys\.go declares no key constant: the check no longer sees it`

import "github.com/MohammadBnei/ukubi-go/errs"

var _ errs.Key
