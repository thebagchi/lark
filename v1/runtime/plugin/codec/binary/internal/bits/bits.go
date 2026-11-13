package bits

import (
	"encoding/binary"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
)

// Decode is bits as bytes, left-padded with zeros to whole bytes first.
//
// Padded rather than refused, because hex reads any count of bits as digits;
// to_bytes refuses a count that is not whole bytes itself. The bits carry no
// prefix: a reader takes 0b off first, since it counts the bits as well.
//
// One pass: each group of eight is checked and read as the same word, into
// bytes made once at their full length. The short group at the front, when the
// count is not whole bytes, is read through _Head, so the padding is never
// built as text.
//
// Returns ErrBits, naming who, for text that is not made only of 0 and 1.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation, from the packing codec's bits2bytes and
//     bits2hex each did
//   - 2026-10-28 23:03: checks and reads each group in one pass, and pads the short
//     group in a word rather than in a copy of the text
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary (C120b, Q315)
func Decode(
	who string,
	bits string,
) ([]byte, error) {
	var (
		short = len(bits) % spec.BITS_PER_BYTE
		data  = make(
			[]byte,
			(len(bits)+spec.BITS_PER_BYTE-1)/spec.BITS_PER_BYTE,
		)
		whole = data
	)

	if short > 0 {
		head := _Head(bits[:short])
		if !_AreBits(head) {
			return nil, common.Refused(
				who,
				bits,
				ErrBits,
			)
		}

		data[0] = _ByteOfBits(head)
		whole = data[1:]
	}

	rest := bits[short:]

	for idx := range whole {
		var (
			at   = idx * spec.BITS_PER_BYTE
			word = _Word(rest[at : at+spec.BITS_PER_BYTE])
		)

		if !_AreBits(word) {
			return nil, common.Refused(
				who,
				bits,
				ErrBits,
			)
		}

		whole[idx] = _ByteOfBits(word)
	}

	return data, nil
}

// _Check refuses text that is not made only of 0 and 1.
//
// Eight characters at a time while eight remain, through _AreBits. The few
// left over are checked one at a time.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's _Bits
//   - 2026-10-03 19:02: eight characters at a time in a word
//   - 2026-10-08 17:47: moved to binary, as _Check
//   - 2026-10-28 23:03: checks a word through _AreBits and refuses through
//     _Refusal, which Decode shares
//   - 2026-11-04 21:49: refuses through common.Refused, which Decode shares (C59)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary (C120b, Q315)
func _Check(
	who string,
	text string,
) error {
	whole := len(text) - len(text)%spec.BITS_PER_BYTE

	for at := 0; at < whole; at += spec.BITS_PER_BYTE {
		if !_AreBits(_Word(text[at : at+spec.BITS_PER_BYTE])) {
			return common.Refused(
				who,
				text,
				ErrBits,
			)
		}
	}

	for _, char := range text[whole:] {
		if char != '0' && char != '1' {
			return common.Refused(
				who,
				text,
				ErrBits,
			)
		}
	}

	return nil
}

// _AreBits reports whether the eight characters in word are all 0 or 1.
//
// The digits 0 and 1 differ only in their lowest bit, so eight characters are
// all digits exactly when every other bit matches ZEROS.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, from the word check in _Check
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary (C120b, Q315)
func _AreBits(word uint64) bool {
	return word&spec.SHARED == spec.ZEROS
}

// _Head is the short group at the front of bits as a word, as if it were
// padded on the left with zeros to eight: the zeros in the low bytes, read
// first, and the characters of head after them in order.
//
// Each character comes in at the top byte and pushes one zero out at the
// bottom, so no padded text is ever made. head is fewer than eight
// characters.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, in place of _Padded
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary (C120b, Q315)
func _Head(head string) uint64 {
	word := uint64(spec.ZEROS)

	for idx := range len(head) {
		word = word>>spec.BITS_PER_BYTE | uint64(head[idx])<<spec.TOP_BYTE
	}

	return word
}

// _ByteOfBits is the byte eight 0/1 characters in word spell, the first most
// significant.
//
// Taking ZEROS away leaves each byte 0 or 1, and one multiply by GATHER moves
// all eight into the word's top byte.
//
// Each character must already be 0 or 1: anything else gives a wrong number,
// not an error. Decode checks the same word through _AreBits first, so the
// refusal names the whole text, not the byte it failed in.
//
// Revisions:
//   - 2026-10-03 17:00: initial creation
//   - 2026-10-03 19:02: folds the eight at once in a word, rather than one at a time,
//     and so takes exactly eight
//   - 2026-10-08 17:47: moved to binary
//   - 2026-10-28 23:03: takes the word, which Decode has already read to check it
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary (C120b, Q315)
func _ByteOfBits(word uint64) byte {
	gathered := (word - spec.ZEROS) * spec.GATHER

	return byte(gathered >> spec.TOP_BYTE)
}

// _Word is eight characters of text as a word, the first in its lowest byte,
// which is the order the masks above read them in.
//
// Revisions:
//   - 2026-10-03 19:02: initial creation
//   - 2026-10-08 17:47: moved to binary
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary (C120b, Q315)
func _Word(text string) uint64 {
	return binary.LittleEndian.Uint64([]byte(text))
}
