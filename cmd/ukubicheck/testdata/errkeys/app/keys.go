// Package app's keys.go is the registry: it makes keys.
package app

import "github.com/MohammadBnei/ukubi-go/errs"

const (
	KeyA errs.Key = "app.a"
	KeyB errs.Key = "app.b"
)

const raw = "app.raw"

var conv = errs.Key("app.c")
