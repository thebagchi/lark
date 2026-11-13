package codec

import (
	"unsafe"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// Encode is data as upper-case hex, two digits a byte, the high digit first,
// with no separators. It is the hex that every part of the runtime writes, so
// a script compares two values it made without a fold of either.
//
// A table of the sixteen upper-case digits, rather than fmt's %X or the lower
// case of encoding/hex and a pass to upper case. In one run of each at 1 KiB,
// %X took 1476 ns, encoding/hex and strings.ToUpper 5450 ns, and this table
// 1120 ns. %X also grows its buffer as it writes: 26 allocations at 64 KiB.
//
// The table loop is EncodeTable. Above a size it gives way to word arithmetic
// (SWAR), and above a larger one to SSSE3, AVX2 or AVX-512 kernels in
// kernel_amd64.s, which the CPU decides at start. Every path writes the same
// bytes; TestKernels_EncodeMatchesTheReference and FuzzKernels_EncodeAgrees say so.
//
// The text is returned as a string over the same bytes, not a copy of them:
// nothing writes text after the loop, and nothing else holds it, which is the
// one condition unsafe.String asks. A copy would be a second allocation of the
// whole output.
//
// Revisions:
//   - 2026-10-29 01:08: initial creation
//   - 2026-11-03 01:11: writes through _EncodeInto, which runs the widest kernel
//     the CPU has (C50)
//   - 2026-11-05 19:07: writes through _EncodeAt (C58)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
func Encode(data []byte) string {
	text := make(
		[]byte,
		len(data)*spec.DIGITS_PER_BYTE,
	)

	EncodeAt(
		cpu.LEVEL,
		text,
		data,
	)

	return unsafe.String(
		unsafe.SliceData(text),
		len(text),
	)
}
