// Package quiet is listed as a wrapper but uses no log: the positive control.
package quiet // want `^-loglevel\.wrappers quiet uses no log: the check no longer sees it`

import "log"

var _ *log.Logger
