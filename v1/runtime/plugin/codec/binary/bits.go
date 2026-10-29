package binary

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

// The masks below read a uint64 as eight bytes side by side, which is how eight
// bits become eight digits, and eight digits one byte, in a handful of
// operations rather than a loop of eight. A word holds eight characters in the
// order they are read: the first in its lowest byte.
const (
	// LANES is 1 in every byte: a byte multiplied by it is a copy in each.
	LANES = 0x0101010101010101

	// ZEROS is the digit 0 in every byte. Added to eight 0s and 1s it makes
	// eight digits; taken from eight digits it leaves eight 0s and 1s.
	ZEROS = 0x3030303030303030

	// SHARED is every bit of a byte that the digits 0 and 1 share, which is all
	// but the lowest: eight characters are all digits exactly when they match
	// ZEROS on these.
	SHARED = 0xFEFEFEFEFEFEFEFE

	// SPREAD keeps one bit of each copy: the highest in the first byte, down to
	// the lowest in the last, so the digits come out most significant first.
	SPREAD = 0x0102040810204080

	// CARRY lifts whatever bit a byte kept into its highest. 0x7F and any one
	// bit add to at least 0x80 and at most 0xFF, so no byte carries into the
	// next.
	CARRY = 0x7F7F7F7F7F7F7F7F

	// GATHER sends the lowest bit of byte k to bit 63-k, so a product's top byte
	// holds eight 0s and 1s as one byte, the first most significant. Every other
	// term of the product lands above bit 63, or on a bit of its own below 56
	// where nothing adds up to carry into the top byte.
	GATHER = 0x8040201008040201

	// TOP_BYTE is where a word's top byte begins: shifted down this far, it
	// stands alone.
	TOP_BYTE = 56
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
func Encode(data []byte) string {
	var (
		text = make(
			[]byte,
			len(data)*BITS_PER_BYTE,
		)
		rest = text
	)

	for _, octet := range data {
		binary.LittleEndian.PutUint64(
			rest,
			_Digits(octet),
		)
		rest = rest[BITS_PER_BYTE:]
	}

	return unsafe.String(
		unsafe.SliceData(text),
		len(text),
	)
}

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
func Decode(
	who string,
	bits string,
) ([]byte, error) {
	var (
		short = len(bits) % BITS_PER_BYTE
		data  = make(
			[]byte,
			(len(bits)+BITS_PER_BYTE-1)/BITS_PER_BYTE,
		)
		whole = data
	)

	if short > 0 {
		head := _Head(bits[:short])
		if !_AreBits(head) {
			return nil, _Refusal(
				who,
				bits,
			)
		}

		data[0] = _ByteOfBits(head)
		whole = data[1:]
	}

	rest := bits[short:]

	for idx := range whole {
		var (
			at   = idx * BITS_PER_BYTE
			word = _Word(rest[at : at+BITS_PER_BYTE])
		)

		if !_AreBits(word) {
			return nil, _Refusal(
				who,
				bits,
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
func _Check(
	who string,
	text string,
) error {
	whole := len(text) - len(text)%BITS_PER_BYTE

	for at := 0; at < whole; at += BITS_PER_BYTE {
		if !_AreBits(_Word(text[at : at+BITS_PER_BYTE])) {
			return _Refusal(
				who,
				text,
			)
		}
	}

	for _, char := range text[whole:] {
		if char != '0' && char != '1' {
			return _Refusal(
				who,
				text,
			)
		}
	}

	return nil
}

// _Refusal is the error for text that is not bits. It names the whole text,
// not the group that failed, so Decode and _Check refuse the same input with
// the same words.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, from the two refusals _Check spelled out
func _Refusal(
	who string,
	text string,
) error {
	return fmt.Errorf(
		"%s got %q: %w",
		who,
		text,
		ErrBits,
	)
}

// _AreBits reports whether the eight characters in word are all 0 or 1.
//
// The digits 0 and 1 differ only in their lowest bit, so eight characters are
// all digits exactly when every other bit matches ZEROS.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, from the word check in _Check
func _AreBits(word uint64) bool {
	return word&SHARED == ZEROS
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
func _Head(head string) uint64 {
	word := uint64(ZEROS)

	for idx := range len(head) {
		word = word>>BITS_PER_BYTE | uint64(head[idx])<<TOP_BYTE
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
func _ByteOfBits(word uint64) byte {
	gathered := (word - ZEROS) * GATHER

	return byte(gathered >> TOP_BYTE)
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
func _Digits(octet byte) uint64 {
	var (
		kept   = (uint64(octet) * LANES) & SPREAD
		lifted = (kept + CARRY) >> (BITS_PER_BYTE - 1)
	)

	return (lifted & LANES) | ZEROS
}

// _Word is eight characters of text as a word, the first in its lowest byte,
// which is the order the masks above read them in.
//
// Revisions:
//   - 2026-10-03 19:02: initial creation
//   - 2026-10-08 17:47: moved to binary
func _Word(text string) uint64 {
	return binary.LittleEndian.Uint64([]byte(text))
}
