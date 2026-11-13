//go:build unix

package tests

import (
	"errors"
	"syscall"
	"testing"
)

// _Reaped fails the test when the process pid is still there: signal 0 finds
// no process once it is gone.
//
// Revisions:
//   - 2026-11-12 18:31: initial creation, the Kill the test called (C128, B284)
func _Reaped(
	t *testing.T,
	pid int,
) {
	t.Helper()

	err := syscall.Kill(
		pid,
		0,
	)
	if !errors.Is(
		err,
		syscall.ESRCH,
	) {
		t.Fatalf(
			"quiet process %d still there after Close: %v",
			pid,
			err,
		)
	}
}
