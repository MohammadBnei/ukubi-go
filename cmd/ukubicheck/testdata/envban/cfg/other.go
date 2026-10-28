package cfg

import "os"

func other() string { return os.Getenv("Y") } // want `^os\.Getenv:`
