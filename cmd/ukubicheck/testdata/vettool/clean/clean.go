// Package clean keeps every rule.
package clean

import (
	"log/slog"

	"example.com/vt/keys"
)

func F() keys.Key {
	slog.Info("clean")
	return keys.A
}
