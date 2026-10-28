// A //line directive does not make a file a test, nor the allowed cfg/config.go.
//
//line lined_test.go:1
package bad

import "os"

func lined() string { return os.Getenv("VT") }
