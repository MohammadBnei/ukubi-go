package dot

import . "syscall" // want `^import \. "syscall":`

func f() { Getenv("X") } // want `^syscall\.Getenv:`
