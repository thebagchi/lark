// Package binary gives a script the module binary: text of 0 and 1, converted
// to and from bytes and integers. Importing it is what enables it.
//
// The text is bits, the most significant first, eight to a byte. A reader
// takes an optional 0b off first, in either case, as a reader of hex takes 0x
// off. Bits and hex convert in the hex module, which reads and writes bits
// through Encode and Decode here.
//
// A module rather than flat names, for the reason base64 is one: from_bytes
// and to_int are words more than one module uses, so each is said behind the
// form it belongs to.
package binary

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ERR_BITS is returned for text that is not made of 0 and 1, or whose length is
// not the multiple of eight that whole bytes need.
var ERR_BITS = errors.New("not a bit string")

const (
	// NAME is the module, and the four names it holds.
	NAME       = "binary"
	FROM_BYTES = "from_bytes"
	TO_BYTES   = "to_bytes"
	FROM_INT   = "from_int"
	TO_INT     = "to_int"

	// BITS_PER_BYTE is how many bits make a whole byte, and BASE what the bits
	// count in.
	BITS_PER_BYTE = 8
	BASE          = 2

	// PAD is the digit bits are padded with, and PREFIX what every reader of
	// bits takes off before reading, in either case.
	PAD    = "0"
	PREFIX = "0b"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
func init() {
	plugin.Register(new(_Binary))
}

// _Binary is the plugin. Empty: every conversion works on what it is given.
type _Binary struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
func (b *_Binary) Name() string {
	return NAME
}

// Values returns the binary module.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
func (b *_Binary) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				FROM_BYTES: starlark.NewBuiltin(NAME+"."+FROM_BYTES, _FromBytes),
				TO_BYTES:   starlark.NewBuiltin(NAME+"."+TO_BYTES, _ToBytes),
				FROM_INT:   starlark.NewBuiltin(NAME+"."+FROM_INT, _FromInt),
				TO_INT:     starlark.NewBuiltin(NAME+"."+TO_INT, _ToInt),
			},
		},
	}
}

// _FromBytes is data as bits, eight to a byte, the most significant first.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bytes2bits
//   - 2026-10-03 17:00: through _BitsOfBytes, which writes each byte from a table
//     rather than building a big.Int for it
//   - 2026-10-08 17:47: a member of the binary module, as from_bytes
func _FromBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(Encode(data)), nil
}

// _ToBytes is the inverse, an optional 0b taken off first; the length must be a
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
func _ToBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	bits := unpack.Unprefixed(text, PREFIX)

	data, err := Decode(fn.Name(), bits)
	if err != nil {
		return nil, err
	}

	if len(bits)%BITS_PER_BYTE != 0 {
		return nil, fmt.Errorf("%s got %d bits, not whole bytes: %w",
			fn.Name(), len(bits), ERR_BITS)
	}

	return starlark.Bytes(data), nil
}

// _FromInt is binary digits left-padded with zeros to the width given, and not
// truncated when the number is wider.
//
// The width is charged to the run before the digits are made: a script names
// it, and Go refuses an allocation it cannot make by ending the process rather
// than the script.
//
// The zeros are added here rather than by a width given to fmt, which refuses
// a width over a million and writes its refusal into the text it returns: the
// call would succeed with the wrong digits.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's int2bits
//   - 2026-10-08 17:47: a member of the binary module, as from_int
//   - 2026-10-08 21:37: charges the width to the run
//   - 2026-10-08 22:19: adds the zeros itself, since fmt refuses a width over a
//     million in the text it returns
func _FromInt(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	number, width, err := unpack.Counted(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	budget := scheduler.Allowance(thread)

	err = budget.Charge(int64(width))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	defer budget.Credit(int64(width))

	bits := number.Text(BASE)
	if len(bits) < width {
		bits = strings.Repeat(PAD, width-len(bits)) + bits
	}

	return starlark.String(bits), nil
}

// _ToInt reads bits as a base-two integer, an optional 0b taken off first.
//
// SetString can refuse only empty bits here, since _Check has let through
// nothing but 0 and 1, and no bits state no number.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bits2int
//   - 2026-10-08 17:47: a member of the binary module, as to_int
//   - 2026-10-08 18:32: takes an optional 0b off first
func _ToInt(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	bits := unpack.Unprefixed(text, PREFIX)

	err = _Check(fn.Name(), bits)
	if err != nil {
		return nil, err
	}

	number, ok := new(big.Int).SetString(bits, BASE)
	if !ok {
		return nil, fmt.Errorf("%s got %q: %w", fn.Name(), text, ERR_BITS)
	}

	return starlark.MakeBigInt(number), nil
}
