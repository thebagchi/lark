//go:build race

package tests

// LIMIT is the largest size of the whole-range sweeps in a build with the race
// detector. The detector does not instrument the assembly, and it slows the Go
// reference about 20 times, so the sweep stops at 600. That is past every block
// (the widest is 64 characters) and past the SSE and AVX2 thresholds (the
// highest of those is 512). The AVX-512 thresholds are 1024, so a race build
// does not reach the shipped AVX-512 path; the hooks that use the smallest
// sizes still run the AVX-512 kernels at every size to 600, under SDE. A normal
// build sweeps to LONG.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
const (
	LIMIT = 600
)
