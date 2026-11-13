// Package cpu says which vector instructions the CPU and the OS give a kernel.
//
// Two packages have kernels in assembly that are chosen at run time by the CPU:
// codec/hex and base64. The detection is the same for both, so it lives here
// once. It is internal to plugin: no script and no host reads it.
//
// Levels, from the narrowest: the portable Go path, SSSE3, AVX2, and AVX-512
// with F and BW. Each includes the ones below it. A build for another CPU, and
// the purego tag, has only the portable level.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package cpu

// Level is the widest set of instructions that a kernel may use. A CPU gets the
// highest level it has, found once at start; a test lowers it to run a
// narrower path on a wide CPU.
//
// Revisions:
//   - 2026-11-03 21:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type Level int

const (
	// LEVEL_SWAR is the portable Go path: no assembly. It is the only level on
	// a CPU that is not amd64, and with the purego tag.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	LEVEL_SWAR Level = iota

	// LEVEL_SSE is SSSE3: 16 bytes in a register, and the byte shuffle.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	LEVEL_SSE

	// LEVEL_AVX2 is 32 bytes in a register.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	LEVEL_AVX2

	// LEVEL_AVX512 is 64 bytes in a register, with AVX-512 F and BW.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	LEVEL_AVX512
)

// LEVEL is the highest level that this CPU supports, found once at start. The
// name is capital snake because the repository's convention check requires it
// of a package-level var; nothing is meant to write it. A write that raises it
// past what the CPU has would fault, as any instruction the CPU lacks does.
//
// Revisions:
//   - 2026-11-03 22:30: moved from codec/hex, to be shared with base64 (C51)
var (
	LEVEL = Detect()
)
