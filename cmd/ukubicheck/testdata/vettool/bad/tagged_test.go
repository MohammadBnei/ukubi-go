//go:build integration

package bad

import (
	"os"
	"testing"
)

func TestTagged(t *testing.T) { _ = os.Getenv("VT") }
