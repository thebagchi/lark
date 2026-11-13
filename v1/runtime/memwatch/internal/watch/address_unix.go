//go:build unix && !openbsd

package watch

import (
	"math"
	"syscall"
)

// _Limit is this process's address-space limit (RLIMIT_AS, ulimit -v), in
// bytes, and whether there is one. An infinite limit is none. The kernel marks
// infinity as the largest value of the type (RLIM_INFINITY): all ones on Linux,
// the largest int64 on macOS, so a value of that size or more counts as none.
// The field is a uint64 on Linux and an int64 on FreeBSD, where infinity is -1:
// the conversion to uint64 makes that a value of the size above too. OpenBSD has
// no RLIMIT_AS, so it is built with address_other.go.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
//   - 2026-11-12 18:22: converts the field to uint64, as FreeBSD has an int64; not built on
//     OpenBSD (C128, B280, B283)
func _Limit() (uint64, bool) {
	var limit syscall.Rlimit

	err := syscall.Getrlimit(
		syscall.RLIMIT_AS,
		&limit,
	)
	if err != nil {
		return 0, false
	}

	cur := uint64(limit.Cur) //nolint:unconvert // an int64 on FreeBSD

	if cur >= math.MaxInt64 {
		return 0, false
	}

	return cur, true
}
