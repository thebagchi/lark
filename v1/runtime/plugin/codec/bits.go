package codec

import (
	"fmt"
	"math/big"
	"strings"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Bytes2Bits is a 0/1 string, most significant bit first, eight bits per
// byte.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
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

	var out strings.Builder

	for _, octet := range data {
		out.WriteString(_Padded(big.NewInt(int64(octet)).Text(BINARY), BITS_PER_BYTE))
	}

	return starlark.String(out.String()), nil
}

// _Bits2Bytes is the inverse; the length must be a multiple of eight, because
// anything else is not whole bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
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
		return nil, fmt.Errorf("%s got %d bits, not whole bytes: %w", fn.Name(), len(text), ErrBits)
	}

	data := make([]byte, 0, len(text)/BITS_PER_BYTE)

	for at := 0; at < len(text); at += BITS_PER_BYTE {
		octet, ok := new(big.Int).SetString(text[at:at+BITS_PER_BYTE], BINARY)
		if !ok {
			return nil, fmt.Errorf("%s got %q: %w", fn.Name(), text, ErrBits)
		}

		data = append(data, byte(octet.Uint64()))
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
		return nil, fmt.Errorf("%s got %q: %w", who, bits, ErrBits)
	}

	return starlark.MakeBigInt(number), nil
}
