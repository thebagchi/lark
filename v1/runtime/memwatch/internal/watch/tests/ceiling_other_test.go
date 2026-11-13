//go:build !unix || openbsd

package tests

import (
	"testing"
)

// _Ceiling is no limit: a system without getrlimit, and OpenBSD, which has no
// RLIMIT_AS, report no address-space limit (see _Limit).
//
// Revisions:
//   - 2026-11-12 18:22: initial creation (C128, B283)
func _Ceiling(t *testing.T) (uint64, bool) {
	t.Helper()

	return 0, false
}
