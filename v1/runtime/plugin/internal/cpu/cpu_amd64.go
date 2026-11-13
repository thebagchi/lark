//go:build amd64 && !purego

package cpu

// The CPUID bits that the levels need. Each comes from the Intel manual, volume
// 2, instruction CPUID, and is the one internal/cpu reads in cpu_x86.go.
//
// Revisions:
//   - 2026-11-03 21:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	// Leaf 1, register ECX.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	CPUID_SSSE3   = 1 << 9
	CPUID_OSXSAVE = 1 << 27
	CPUID_AVX     = 1 << 28

	// Leaf 7 sub-leaf 0, register EBX.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	CPUID_AVX2     = 1 << 5
	CPUID_AVX512F  = 1 << 16
	CPUID_AVX512BW = 1 << 30

	// CPUID_AVX512_NEED is the pair of bits the AVX-512 kernels need.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	CPUID_AVX512_NEED = CPUID_AVX512F | CPUID_AVX512BW

	// XCR0: the state that the OS saves on a switch. Bits 1 and 2 are the XMM
	// and YMM state. Bits 5, 6 and 7 are the mask registers and the ZMM state.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	XCR0_AVX    = 0x6
	XCR0_AVX512 = 0xe0

	// The first leaf that holds the extended feature bits.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	CPUID_LEAF_FEATURES = 7
)

// _Cpuid runs the CPUID instruction for the leaf and sub-leaf given.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//
//go:noescape
func _Cpuid(
	leaf uint32,
	sub uint32,
) (eax uint32, ebx uint32, ecx uint32, edx uint32)

// _Xgetbv reads XCR0. Call it only when OSXSAVE is set.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//
//go:noescape
func _Xgetbv() (eax uint32, edx uint32)

// Detect is the highest level that this CPU and this OS support.
//
// The standard library's internal/cpu has these flags, but only the standard
// library may import it. golang.org/x/sys/cpu is a new dependency, which this
// module does not take. So this reads CPUID itself, as internal/cpu does. A
// CPU feature is in use only when the OS saves its state: the XCR0 check is
// what keeps a virtual machine that hides AVX from a fault.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:30: moved from codec/hex, to be shared with base64 (C51)
//   - 2026-11-04 06:44: doc comment wrapped to the file width (C51 fix round 1)
func Detect() Level {
	var maxLeaf, _, _, _ = _Cpuid(
		0,
		0,
	)
	if maxLeaf < CPUID_LEAF_FEATURES {
		return LEVEL_SWAR
	}

	var _, _, ecx1, _ = _Cpuid(
		1,
		0,
	)
	if ecx1&CPUID_SSSE3 == 0 {
		return LEVEL_SWAR
	}

	level := LEVEL_SSE

	if ecx1&(CPUID_OSXSAVE|CPUID_AVX) != CPUID_OSXSAVE|CPUID_AVX {
		return level
	}

	var xcr0, _ = _Xgetbv()
	if xcr0&XCR0_AVX != XCR0_AVX {
		return level
	}

	var _, ebx7, _, _ = _Cpuid(
		CPUID_LEAF_FEATURES,
		0,
	)
	if ebx7&CPUID_AVX2 == 0 {
		return level
	}

	level = LEVEL_AVX2

	if xcr0&XCR0_AVX512 == XCR0_AVX512 && ebx7&CPUID_AVX512_NEED == CPUID_AVX512_NEED {
		level = LEVEL_AVX512
	}

	return level
}
