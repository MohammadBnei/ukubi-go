// Package unix stubs golang.org/x/sys/unix's environment readers.
package unix

func Getenv(string) (string, bool)          { return "", false }
func Environ() []string                     { return nil }
func Setenv(string, string) error           { return nil }
func Unsetenv(string) error                 { return nil }
func Clearenv()                             {}
func Exec(string, []string, []string) error { return nil }
func Getwd() (string, error)                { return "", nil }
func Getpid() int                           { return 0 }
