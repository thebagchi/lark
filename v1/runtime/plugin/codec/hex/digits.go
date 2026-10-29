package hex

import (
	"unsafe"
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
// The text is returned as a string over the same bytes, not a copy of them:
// nothing writes text after the loop, and nothing else holds it, which is the
// one condition unsafe.String asks. A copy would be a second allocation of the
// whole output.
//
// Revisions:
//   - 2026-10-29 01:08: initial creation
func Encode(data []byte) string {
	text := make(
		[]byte,
		len(data)*DIGITS_PER_BYTE,
	)

	for idx, octet := range data {
		pair := text[idx*DIGITS_PER_BYTE:]
		pair[0] = UPPER[octet>>BITS_PER_DIGIT]
		pair[1] = UPPER[octet&LOW_DIGIT]
	}

	return unsafe.String(
		unsafe.SliceData(text),
		len(text),
	)
}
