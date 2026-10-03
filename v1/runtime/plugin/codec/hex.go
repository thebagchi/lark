package codec

import (
	"encoding/hex"
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Bytes2Hex is lowercase hex with no separators.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Bytes2Hex(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(hex.EncodeToString(data)), nil
}

// _Hex2Bytes is the inverse; an odd number of digits is an error, because it
// is not a whole number of bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Hex2Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	data, err := hex.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("%s got %q: %w", fn.Name(), text, ERR_HEX)
	}

	return starlark.Bytes(data), nil
}

// _Bits2Hex is uppercase hex, the bits left-padded to a multiple of four
// first so the last digit is whole.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Bits2Hex(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _HexOfBits(fn.Name(), text)
}

// _HexOfBits is bits as upper-case hex, left-padded to whole nibbles first so
// the last digit is whole.
//
// The bits are read a byte at a time, two digits each, which pads them to whole
// bytes. That is one nibble more than whole nibbles when their count is odd,
// and the extra is a zero digit in front, which is dropped.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-10-03 17:00: each nibble through _ByteOfBits and UPPER_HEX rather than a
//     big.Int; the characters are already checked, so a nibble cannot fail to
//     parse
//   - 2026-10-03 19:02: a byte at a time, two digits each, dropping the zero digit whole
//     bytes add in front of an odd count of nibbles
func _HexOfBits(who string, bits string) (starlark.Value, error) {
	err := _Bits(who, bits)
	if err != nil {
		return nil, err
	}

	padded := _Padded(bits, BITS_PER_BYTE)
	out := make([]byte, 0, len(padded)/BITS_PER_NIBBLE)

	for at := 0; at < len(padded); at += BITS_PER_BYTE {
		octet := _ByteOfBits(padded[at : at+BITS_PER_BYTE])
		out = append(out, UPPER_HEX[octet/HEXADECIMAL], UPPER_HEX[octet%HEXADECIMAL])
	}

	// A digit for every four bits, the last perhaps short of four.
	digits := (len(bits) + BITS_PER_NIBBLE - 1) / BITS_PER_NIBBLE

	return starlark.String(out[len(out)-digits:]), nil
}

// _Hex2Bits is four bits per digit, an optional 0x prefix stripped, the result
// left-padded to a multiple of eight so it reads as whole bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Hex2Bits(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	bits, err := _BitsOfHex(fn.Name(), text)
	if err != nil {
		return nil, err
	}

	return starlark.String(bits), nil
}

// _BitsOfHex is hex digits as bits, an optional 0x prefix stripped and the
// result left-padded to whole bytes: the digits decoded as bytes, and those
// bytes as bits.
//
// The digits are padded to whole bytes before decoding rather than the bits
// after, and the two are one rule: an odd count gains one zero digit, which is
// the four zero bits padding the bits to a byte would add.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-10-03 17:00: decodes the digits with encoding/hex rather than a big.Int for
//     each one
func _BitsOfHex(who string, text string) (string, error) {
	digits := strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")

	data, err := hex.DecodeString(_Padded(digits, DIGITS_PER_BYTE))
	if err != nil {
		return "", fmt.Errorf("%s got %q: %w", who, text, ERR_HEX)
	}

	return _BitsOfBytes(data), nil
}

// _Int2Hex is int2bits then bits2hex, as the brief defines it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Int2Hex(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	number, width, err := _Counted(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _HexOfBits(fn.Name(), _BitsOfInt(number, width))
}

// _Hex2Int is hex2bits then bits2int, as the brief defines it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Hex2Int(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	bits, err := _BitsOfHex(fn.Name(), text)
	if err != nil {
		return nil, err
	}

	return _IntOfBits(fn.Name(), bits)
}
