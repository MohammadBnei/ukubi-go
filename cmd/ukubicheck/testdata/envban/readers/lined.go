// A //line directive does not make a file a test: its real name is lined.go.
//
//line lined_test.go:1
package readers

import "os"

func lined() string { return os.Getenv("HOME") } // want `^os\.Getenv:`
