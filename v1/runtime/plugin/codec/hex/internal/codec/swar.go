package codec

import (
	stdbinary "encoding/binary"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
)

// EncodeSWAR writes the upper-case hex of src to dst, four bytes a step, with
// no table. dst must hold 2*len(src) bytes.
//
// A step spreads the four bytes to four lanes of 16 bits, moves the low
// nibble of each into the high byte of its lane, and adds 0x30 to every nibble,
// and 7 more to each that is 10 or more. The compare is an add of 6: a nibble
// of 10 or more carries into bit 4. All of it is word arithmetic on one
// uint64, so the loop has no table load and no branch on the data.
//
// The bytes that fewer than four leave are written by the table loop.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex and renamed _EncodeSWAR to
//     EncodeSWAR, for the tests in tests/ (C120b, Q315, Q314)
func EncodeSWAR(
	dst []byte,
	src []byte,
) {
	var idx int

	for ; idx+spec.SWAR_IN <= len(src); idx += spec.SWAR_IN {
		word := uint64(stdbinary.LittleEndian.Uint32(src[idx:]))

		stdbinary.LittleEndian.PutUint64(
			dst[idx*spec.DIGITS_PER_BYTE:],
			_DigitsOfLanes(word),
		)
	}

	EncodeTable(
		dst[idx*spec.DIGITS_PER_BYTE:],
		src[idx:],
	)
}

// _DigitsOfLanes is the eight upper-case digits of the four bytes in the low
// half of word, the digit of the first byte's high nibble in the lowest byte.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
func _DigitsOfLanes(word uint64) uint64 {
	word = (word | word<<spec.SHIFT_LANE16) & spec.SWAR_LANE16
	word = (word | word<<spec.SHIFT_BYTE) & spec.SWAR_LANE8

	var (
		high   = (word >> spec.BITS_PER_DIGIT) & spec.SWAR_LANE4
		low    = (word & spec.SWAR_LANE4) << spec.SHIFT_BYTE
		nibble = high | low
		carry  = ((nibble + spec.SWAR_ADD_SIX) >> spec.CARRY_SHIFT) & spec.SWAR_ONES
	)

	return nibble + spec.SWAR_ASCII_ZERO + carry*spec.LETTER_GAP
}

// EncodeTable writes the upper-case hex of src to dst, a byte at a step. It is
// the loop that Encode had before C50, and the tail of the wider paths.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation, from Encode's loop (C50)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex and renamed _EncodeTable to
//     EncodeTable, for the tests in tests/ (C120b, Q315, Q314)
func EncodeTable(
	dst []byte,
	src []byte,
) {
	for idx, octet := range src {
		pair := dst[idx*spec.DIGITS_PER_BYTE:]
		pair[0] = spec.UPPER[octet>>spec.BITS_PER_DIGIT]
		pair[1] = spec.UPPER[octet&spec.LOW_DIGIT]
	}
}
