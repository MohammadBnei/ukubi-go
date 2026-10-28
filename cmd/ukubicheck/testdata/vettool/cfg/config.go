// Package cfg's config.go is the -envban.allow file.
package cfg

import "os"

func Load() string { return os.Getenv("VT") }
