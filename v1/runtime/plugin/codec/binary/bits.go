package binary

import (
	"encoding/binary"
	"fmt"
	"strings"
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
// Each byte's eight digits are made at once in a word, by _Digits, and written
// together. A table of the sixteen nibbles, which this replaced, ran at 214
// MB/s, and this at 450.
//
// Revisions:
//   - 2026-10-03 17:00: initial creation, as codec's _BitsOfBytes
//   - 2026-10-03 19:02: makes each byte's eight digits at once in a word, rather than
//     looking them up a nibble at a time
//   - 2026-10-08 17:47: moved to binary and exported, so hex writes bits through it too
func Encode(data []byte) string {
	var (
		out    strings.Builder
		digits [BITS_PER_BYTE]byte
	)

	out.Grow(len(data) * BITS_PER_BYTE)

	for _, octet := range data {
		binary.LittleEndian.PutUint64(digits[:], _Digits(octet))
		out.Write(digits[:])
	}

	return out.String()
}

// Decode is bits as bytes, left-padded with zeros to whole bytes first.
//
// Padded rather than refused, because hex reads any count of bits as digits;
// to_bytes refuses a count that is not whole bytes itself. The bits carry no
// prefix: a reader takes 0b off first, since it counts the bits as well.
//
// Returns ERR_BITS, naming who, for text that is not made only of 0 and 1.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation, from the packing codec's bits2bytes and
//     bits2hex each did
func Decode(who string, bits string) ([]byte, error) {
	err := _Check(who, bits)
	if err != nil {
		return nil, err
	}

	padded := _Padded(bits)
	data := make([]byte, 0, len(padded)/BITS_PER_BYTE)

	for at := 0; at < len(padded); at += BITS_PER_BYTE {
		data = append(data, _ByteOfBits(padded[at:at+BITS_PER_BYTE]))
	}

	return data, nil
}

// _Check refuses text that is not made only of 0 and 1.
//
// Eight characters at a time while eight remain: the digits 0 and 1 differ only
// in their lowest bit, so eight characters are all digits exactly when every
// other bit matches ZEROS. The few left over are checked one at a time.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's _Bits
//   - 2026-10-03 19:02: eight characters at a time in a word
//   - 2026-10-08 17:47: moved to binary, as _Check
func _Check(who string, text string) error {
	whole := len(text) - len(text)%BITS_PER_BYTE

	for at := 0; at < whole; at += BITS_PER_BYTE {
		digits := _Word(text[at:at+BITS_PER_BYTE])&SHARED == ZEROS
		if !digits {
			return fmt.Errorf("%s got %q: %w", who, text, ERR_BITS)
		}
	}

	for _, char := range text[whole:] {
		if char != '0' && char != '1' {
			return fmt.Errorf("%s got %q: %w", who, text, ERR_BITS)
		}
	}

	return nil
}

// _Padded is bits left-padded with zeros to whole bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's _Padded, to any width
//   - 2026-10-08 17:47: to whole bytes, the one width left that bits are padded to
func _Padded(bits string) string {
	short := len(bits) % BITS_PER_BYTE
	if short == 0 {
		return bits
	}

	return strings.Repeat(PAD, BITS_PER_BYTE-short) + bits
}

// _ByteOfBits is the byte eight 0/1 characters spell, the first most
// significant.
//
// The eight are read as one word, taking ZEROS away leaves each byte 0 or 1,
// and one multiply by GATHER moves all eight into the word's top byte.
//
// Each character must already be 0 or 1: anything else gives a wrong number,
// not an error. A check here could only name the byte it failed in, where a
// check of the whole string names all of it.
//
// Revisions:
//   - 2026-10-03 17:00: initial creation
//   - 2026-10-03 19:02: folds the eight at once in a word, rather than one at a time,
//     and so takes exactly eight
//   - 2026-10-08 17:47: moved to binary
func _ByteOfBits(bits string) byte {
	gathered := (_Word(bits) - ZEROS) * GATHER

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
	kept := (uint64(octet) * LANES) & SPREAD
	lifted := (kept + CARRY) >> (BITS_PER_BYTE - 1)

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
