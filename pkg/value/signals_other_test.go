//go:build !unix

package value

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// TestSendSignalOnAPlatformWithoutThem pins the two answers windows gives process.kill.
// Signal zero is the liveness check and sends nothing, so it succeeds for a process that
// is there, and a signal the platform has a name for and no way to raise says so rather
// than quietly doing nothing. The signals that terminate are left out on purpose: there
// is no way to test one without ending a process.
func TestSendSignalOnAPlatformWithoutThem(t *testing.T) {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find this process: %v", err)
	}
	if err := sendSignal(p, 0); err != nil {
		t.Errorf("signal 0 to this process answered %v, want no error", err)
	}
	if err := sendSignal(p, syscall.SIGHUP); !errors.Is(err, errSignalUnsupported) {
		t.Errorf("SIGHUP answered %v, want errSignalUnsupported", err)
	}
}
