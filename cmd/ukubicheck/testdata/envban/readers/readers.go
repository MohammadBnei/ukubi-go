// Package readers has each reader once, as a value, and a call.
package readers

import (
	"io/ioutil"
	"net/http"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func values() {
	_ = os.Getenv                 // want `^os\.Getenv: only the config file`
	_ = os.LookupEnv              // want `^os\.LookupEnv:`
	_ = os.Environ                // want `^os\.Environ:`
	_ = os.ExpandEnv              // want `^os\.ExpandEnv:`
	_ = os.Setenv                 // want `^os\.Setenv:`
	_ = os.Unsetenv               // want `^os\.Unsetenv:`
	_ = os.Clearenv               // want `^os\.Clearenv:`
	_ = os.UserHomeDir            // want `^os\.UserHomeDir:`
	_ = os.UserCacheDir           // want `^os\.UserCacheDir:`
	_ = os.UserConfigDir          // want `^os\.UserConfigDir:`
	_ = os.TempDir                // want `^os\.TempDir:`
	_ = os.MkdirTemp              // want `^os\.MkdirTemp:`
	_ = os.CreateTemp             // want `^os\.CreateTemp:`
	_ = os.Getwd                  // want `^os\.Getwd:`
	_ = os.StartProcess           // want `^os\.StartProcess:`
	_ = ioutil.TempDir            // want `^io/ioutil\.TempDir:`
	_ = ioutil.TempFile           // want `^io/ioutil\.TempFile:`
	_ = syscall.Getenv            // want `^syscall\.Getenv:`
	_ = syscall.Environ           // want `^syscall\.Environ:`
	_ = syscall.Setenv            // want `^syscall\.Setenv:`
	_ = syscall.Unsetenv          // want `^syscall\.Unsetenv:`
	_ = syscall.Clearenv          // want `^syscall\.Clearenv:`
	_ = syscall.ForkExec          // want `^syscall\.ForkExec:`
	_ = syscall.Exec              // want `^syscall\.Exec:`
	_ = syscall.StartProcess      // want `^syscall\.StartProcess:`
	_ = unix.Getenv               // want `^golang\.org/x/sys/unix\.Getenv:`
	_ = unix.Environ              // want `^golang\.org/x/sys/unix\.Environ:`
	_ = unix.Setenv               // want `^golang\.org/x/sys/unix\.Setenv:`
	_ = unix.Unsetenv             // want `^golang\.org/x/sys/unix\.Unsetenv:`
	_ = unix.Clearenv             // want `^golang\.org/x/sys/unix\.Clearenv:`
	_ = unix.Exec                 // want `^golang\.org/x/sys/unix\.Exec:`
	_ = http.ProxyFromEnvironment // want `^net/http\.ProxyFromEnvironment:`
}

func call() string { return os.Getenv("HOME") } // want `^os\.Getenv:`

func notReaders() { _, _ = os.Args, unix.Getpid() }
