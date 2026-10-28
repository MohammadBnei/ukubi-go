// Package wrap is a wrapper: it may use log.
package wrap

import "log"

func Print(s string) { log.Print(s) }
