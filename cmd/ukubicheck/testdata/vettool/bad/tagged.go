//go:build integration

package bad

import o "os"

func tagged() string { return o.Getenv("VT") }
