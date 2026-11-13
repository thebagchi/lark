package binary

import (
	"encoding/binary"
	"unsafe"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/bits"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/spec"
)

// Encode is data as bits, eight characters 0 and 1 to a byte, the most
// significant bit first.
//
// Each byte's eight digits are made at once in a word, by _Digits, and stored
// in place in text made once at its full length. A table of the sixteen
// nibbles, which this replaced, ran at 214 MB/s, and this at 450.
//
// The text is returned as a string over the same bytes, not a copy of them:
// nothing writes text after the loop, and nothing else holds it, which is the
// one condition unsafe.String asks. A copy would be a second allocation of the
// whole output; strings.Builder avoids that the same way.
//
// Revisions:
//   - 2026-10-03 17:00: initial creation, as codec's _BitsOfBytes
//   - 2026-10-03 19:02: makes each byte's eight digits at once in a word, rather than
//     looking them up a nibble at a time
//   - 2026-10-08 17:47: moved to binary and exported, so hex writes bits through it too
//   - 2026-10-28 23:03: stores each word in place in text made once, rather than
//     writing it through a strings.Builder
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func Encode(data []byte) string {
	var (
		text = make(
			[]byte,
			len(data)*spec.BITS_PER_BYTE,
		)
		rest = text
	)

	for _, octet := range data {
		binary.LittleEndian.PutUint64(
			rest,
			_Digits(octet),
		)
		rest = rest[spec.BITS_PER_BYTE:]
	}

	return unsafe.String(
		unsafe.SliceData(text),
		len(text),
	)
}

// Decode calls bits.Decode.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q319)
func Decode(
	who string,
	text string,
) ([]byte, error) {
	return bits.Decode(
		who,
		text,
	)
}

// _Digits is octet's eight bits as eight characters 0 and 1 in a word, the
// most significant in its lowest byte, which is the one written first.
//
// The byte is copied into every byte of the word, each copy keeps a different
// one of its bits, and each kept bit becomes 0 or 1 and then a digit: eight
// digits in six operations, where a loop takes eight passes.
//
// Revisions:
//   - 2026-10-03 19:02: initial creation
//   - 2026-10-08 17:47: moved to binary
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func _Digits(octet byte) uint64 {
	var (
		kept   = (uint64(octet) * spec.LANES) & spec.SPREAD
		lifted = (kept + spec.CARRY) >> (spec.BITS_PER_BYTE - 1)
	)

	return (lifted & spec.LANES) | spec.ZEROS
}
