//go:build unix && !openbsd

package tests

import (
	"math"
	"syscall"
	"testing"
)

// _Ceiling is the address-space limit of this process (RLIMIT_AS), and whether
// there is one. An infinite limit is none.
//
// Revisions:
//   - 2026-11-12 18:22: initial creation, from TestAddress_ReadsTheRealLimit (C128, B280, B283)
func _Ceiling(t *testing.T) (uint64, bool) {
	t.Helper()

	var limit syscall.Rlimit

	err := syscall.Getrlimit(
		syscall.RLIMIT_AS,
		&limit,
	)
	if err != nil {
		t.Fatal(err)
	}

	cur := uint64(limit.Cur) //nolint:unconvert // an int64 on FreeBSD

	return cur, cur < math.MaxInt64
}
