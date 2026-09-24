package codec

import (
	"fmt"
	"math/big"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/plugin/unpack"
)

// _Bytes2Int is an unsigned big-endian integer.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Bytes2Int(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.MakeBigInt(new(big.Int).SetBytes(data)), nil
}

// _Int2Bytes is unsigned big-endian, zero-padded to the width given; the
// number must be non-negative and must fit.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Int2Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	number, width, err := _Counted(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	if number.BitLen() > width*BITS_PER_BYTE {
		return nil, fmt.Errorf("%s: %s does not fit %d bytes: %w", fn.Name(), number, width, ErrRange)
	}

	return starlark.Bytes(number.FillBytes(make([]byte, width))), nil
}
