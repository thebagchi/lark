// Package bits holds the bit-string conversions of the binary plugin: the check
// that a text is made of bits, its decoding to bytes and to an integer, and the
// sentinel for a text that is not.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package bits

import (
	"errors"
	"fmt"
	"math/big"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrBits is returned for text that is not made of 0 and 1, or whose length is
// not the multiple of eight that whole bytes need.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary (C120b, Q315)
var (
	ErrBits = errors.New("not a bit string")
)

// ToBytes is the inverse, an optional 0b taken off first; the length must be a
// multiple of eight, because anything else is not whole bytes.
//
// Decoded before the length is checked, so text that is not bits at all is
// refused as that, rather than as bits that fall short of a byte.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bits2bytes
//   - 2026-10-03 17:00: each byte through _ByteOfBits rather than a big.Int; the
//     characters are already checked, so a byte cannot fail to parse
//   - 2026-10-08 17:47: a member of the binary module, as to_bytes, reading the bits
//     through Decode
//   - 2026-10-08 18:32: takes an optional 0b off first
//   - 2026-11-03 00:30: hands the decoded bytes over, with no copy of them (C37)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:59: names from spec and common, helpers as shared (C91w12c, Q228, Q244)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary and renamed _ToBytes to
//     ToBytes, for the tests in tests/ (C120b, Q315, Q314)
func ToBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	bits := unpack.Unprefixed(
		text,
		common.BITS_PREFIX,
	)

	var data []byte

	data, err = Decode(
		shared.Called(fn),
		bits,
	)
	if err != nil {
		return nil, err
	}

	if len(bits)%spec.BITS_PER_BYTE != 0 {
		return nil, fmt.Errorf(
			"%s got %d bits, not whole bytes: %w",
			shared.Called(fn),
			len(bits),
			ErrBits,
		)
	}

	return starlark.Bytes(unpack.Handover(data)), nil
}

// ToInt reads bits as a base-two integer, an optional 0b taken off first.
//
// SetString can refuse only empty bits here, since _Check has let through
// nothing but 0 and 1, and no bits state no number.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bits2int
//   - 2026-10-08 17:47: a member of the binary module, as to_int
//   - 2026-10-08 18:32: takes an optional 0b off first
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:59: names from spec and common, helpers as shared (C91w12c, Q228, Q244)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/binary and renamed _ToInt to ToInt,
//     for the tests in tests/ (C120b, Q315, Q314)
func ToInt(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	bits := unpack.Unprefixed(
		text,
		common.BITS_PREFIX,
	)

	err = _Check(
		shared.Called(fn),
		bits,
	)
	if err != nil {
		return nil, err
	}

	number, ok := new(big.Int).SetString(
		bits,
		spec.BASE,
	)
	if !ok {
		return nil, shared.Refused(
			shared.Called(fn),
			text,
			ErrBits,
		)
	}

	return starlark.MakeBigInt(number), nil
}
