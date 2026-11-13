//go:build !unix

package tests

import (
	"testing"
)

// _Reaped skips the test: a system that is not a unix has no signal 0 to find
// a process with, and no /bin/sh for the plugin the test runs.
//
// Revisions:
//   - 2026-11-12 18:31: initial creation (C128, B284)
func _Reaped(
	t *testing.T,
	_ int,
) {
	t.Helper()

	t.Skip("no signal 0 here")
}
