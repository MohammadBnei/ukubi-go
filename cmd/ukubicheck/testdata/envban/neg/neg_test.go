package neg

import (
	"os"
	"os/exec"
	"testing"
)

func TestReads(t *testing.T) { _, _ = os.Getenv("X"), exec.Command }
