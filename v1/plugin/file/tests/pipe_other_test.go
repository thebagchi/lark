//go:build !unix

package tests

import (
	"testing"
)

// _Pipe skips the test: a system that is not a unix has no named pipe to make
// with a path.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B277)
func _Pipe(
	t *testing.T,
	_ string,
) {
	t.Helper()

	t.Skip("no named pipes here")
}

// _Release does nothing: _Pipe skips every test that would need it.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B277)
func _Release(_ string) {}
