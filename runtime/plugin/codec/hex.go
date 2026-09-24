package codec

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/plugin/unpack"
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
		return nil, fmt.Errorf("%s got %q: %w", fn.Name(), text, ErrHex)
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

// _HexOfBits is the conversion _Bits2Hex and _Int2Hex share.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _HexOfBits(who string, bits string) (starlark.Value, error) {
	err := _Bits(who, bits)
	if err != nil {
		return nil, err
	}

	padded := _Padded(bits, BITS_PER_NIBBLE)

	var out strings.Builder

	for at := 0; at < len(padded); at += BITS_PER_NIBBLE {
		nibble, ok := new(big.Int).SetString(padded[at:at+BITS_PER_NIBBLE], BINARY)
		if !ok {
			return nil, fmt.Errorf("%s got %q: %w", who, bits, ErrBits)
		}

		out.WriteString(strings.ToUpper(nibble.Text(HEXADECIMAL)))
	}

	return starlark.String(out.String()), nil
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

// _BitsOfHex is the conversion _Hex2Bits and _Hex2Int share.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _BitsOfHex(who string, text string) (string, error) {
	digits := strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")

	var out strings.Builder

	for _, char := range digits {
		digit, ok := new(big.Int).SetString(string(char), HEXADECIMAL)
		if !ok {
			return "", fmt.Errorf("%s got %q: %w", who, text, ErrHex)
		}

		out.WriteString(_Padded(digit.Text(BINARY), BITS_PER_NIBBLE))
	}

	return _Padded(out.String(), BITS_PER_BYTE), nil
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
