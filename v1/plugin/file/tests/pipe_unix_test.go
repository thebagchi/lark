//go:build unix

package tests

import (
	"os"
	"syscall"
	"testing"
)

// _Pipe makes a named pipe at path, and skips the test when the system cannot.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation, the Mkfifo the tests called (C128, B277)
func _Pipe(
	t *testing.T,
	path string,
) {
	t.Helper()

	err := syscall.Mkfifo(
		path,
		0o600,
	)
	if err != nil {
		t.Skipf(
			"no named pipes here: %v",
			err,
		)
	}
}

// _Release opens the named pipe at path for reading without waiting, so a
// goroutine blocked in open(2) for writing ends.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation, the open the tests made (C128, B277)
func _Release(path string) {
	reader, err := os.OpenFile(
		path,
		os.O_RDONLY|syscall.O_NONBLOCK,
		0,
	)
	if err == nil {
		_ = reader.Close()
	}
}
