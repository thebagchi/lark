package codec

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
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

// _Bytes2Bits is a 0/1 string, most significant bit first, eight bits per
// byte.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-10-03 17:00: through _BitsOfBytes, which writes each byte from a table
//     rather than building a big.Int for it
func _Bytes2Bits(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(_BitsOfBytes(data)), nil
}

// _Bits2Bytes is the inverse; the length must be a multiple of eight, because
// anything else is not whole bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-10-03 17:00: each byte through _ByteOfBits rather than a big.Int; the
//     characters are already checked, so a byte cannot fail to parse
func _Bits2Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	err = _Bits(fn.Name(), text)
	if err != nil {
		return nil, err
	}

	if len(text)%BITS_PER_BYTE != 0 {
		return nil, fmt.Errorf("%s got %d bits, not whole bytes: %w",
			fn.Name(), len(text), ERR_BITS)
	}

	data := make([]byte, 0, len(text)/BITS_PER_BYTE)

	for at := 0; at < len(text); at += BITS_PER_BYTE {
		data = append(data, _ByteOfBits(text[at:at+BITS_PER_BYTE]))
	}

	return starlark.Bytes(data), nil
}

// _Int2Bits is binary digits left-padded to the width given, and not
// truncated when the number is wider.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Int2Bits(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	number, width, err := _Counted(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(_BitsOfInt(number, width)), nil
}

// _BitsOfInt is the conversion _Int2Bits and _Int2Hex share.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _BitsOfInt(number *big.Int, width int) string {
	bits := number.Text(BINARY)

	if len(bits) >= width {
		return bits
	}

	return strings.Repeat("0", width-len(bits)) + bits
}

// _Bits2Int parses a bit string as a base-two integer.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Bits2Int(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _IntOfBits(fn.Name(), text)
}

// _IntOfBits is the conversion _Bits2Int and _Hex2Int share.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _IntOfBits(who string, bits string) (starlark.Value, error) {
	err := _Bits(who, bits)
	if err != nil {
		return nil, err
	}

	number, ok := new(big.Int).SetString(bits, BINARY)
	if !ok {
		return nil, fmt.Errorf("%s got %q: %w", who, bits, ERR_BITS)
	}

	return starlark.MakeBigInt(number), nil
}

// _BitsOfBytes is each byte as eight 0/1 characters, most significant bit
// first.
//
// Each byte's eight digits are made at once in a word, by _Digits, and written
// together. A table of the sixteen nibbles, which this replaced, ran at 214
// MB/s, and this at 450.
//
// Revisions:
//   - 2026-10-03 17:00: initial creation
//   - 2026-10-03 19:02: makes each byte's eight digits at once in a word, rather than
//     looking them up a nibble at a time
func _BitsOfBytes(data []byte) string {
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
func _Word(text string) uint64 {
	return binary.LittleEndian.Uint64([]byte(text))
}
