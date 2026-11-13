//go:build !unix || openbsd

package watch

// _Limit is no limit on a system without getrlimit, and on OpenBSD. OpenBSD has
// no limit on the address space (no RLIMIT_AS); its RLIMIT_DATA limits the data
// segment only, which is not what Address reports, so it is not read. A run
// there is bounded by the memory of the system alone, as on Windows.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
//   - 2026-11-12 18:22: also built on OpenBSD, which has no address-space limit (C128, B283)
func _Limit() (uint64, bool) {
	return 0, false
}
