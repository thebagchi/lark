// Package hex gives a script the module hex: hexadecimal text, converted to and
// from bytes, integers and bits. Importing it is what enables it.
//
// Hex is written in upper case and read in either. One case out lets a script
// compare two values it made without folding either; either case in lets it
// read what something else wrote. A reader of hex takes an optional 0x off
// first, and from_binary an optional 0b, in either case, for the same reason.
//
// Bits are read and written through the binary package, so importing this one
// enables binary as well.
package hex

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ERR_HEX is returned for text that is not hexadecimal, or has an odd number of
// digits where whole bytes are wanted.
var ERR_HEX = errors.New("not hexadecimal")

const (
	// NAME is the module, and the six names it holds.
	NAME        = "hex"
	FROM_BYTES  = "from_bytes"
	TO_BYTES    = "to_bytes"
	FROM_INT    = "from_int"
	TO_INT      = "to_int"
	FROM_BINARY = "from_binary"
	TO_BINARY   = "to_binary"

	// BITS_PER_DIGIT is how many bits one digit holds, DIGITS_PER_BYTE how
	// many digits make a whole byte, and BASE what the digits count in.
	BITS_PER_DIGIT  = 4
	DIGITS_PER_BYTE = 2
	BASE            = 16

	// PREFIX is what every reader here takes off before reading, in either
	// case, and PAD the digit an odd count of digits is padded with.
	PREFIX = "0x"
	PAD    = "0"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
func init() {
	plugin.Register(new(_Hex))
}

// _Hex is the plugin. Empty: every conversion works on what it is given.
type _Hex struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
func (h *_Hex) Name() string {
	return NAME
}

// Values returns the hex module.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
func (h *_Hex) Values() starlark.StringDict {
	members := starlark.StringDict{
		FROM_BYTES:  starlark.NewBuiltin(NAME+"."+FROM_BYTES, _FromBytes),
		TO_BYTES:    starlark.NewBuiltin(NAME+"."+TO_BYTES, _ToBytes),
		FROM_INT:    starlark.NewBuiltin(NAME+"."+FROM_INT, _FromInt),
		TO_INT:      starlark.NewBuiltin(NAME+"."+TO_INT, _ToInt),
		FROM_BINARY: starlark.NewBuiltin(NAME+"."+FROM_BINARY, _FromBinary),
		TO_BINARY:   starlark.NewBuiltin(NAME+"."+TO_BINARY, _ToBinary),
	}

	return starlark.StringDict{
		NAME: &starlarkstruct.Module{Name: NAME, Members: members},
	}
}

// _FromBytes is data as hex, two digits a byte, with no separators.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bytes2hex
//   - 2026-10-08 17:47: a member of the hex module, as from_bytes, writing upper case
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

	return starlark.String(fmt.Sprintf("%X", data)), nil
}

// _ToBytes is the inverse, an optional 0x taken off first; an odd number of
// digits is an error, because it is not a whole number of bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's hex2bytes
//   - 2026-10-08 17:47: a member of the hex module, as to_bytes
//   - 2026-10-08 18:06: takes an optional 0x off first, as to_int and to_binary do
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

	data, err := hex.DecodeString(unpack.Unprefixed(text, PREFIX))
	if err != nil {
		return nil, fmt.Errorf("%s got %q: %w", fn.Name(), text, ERR_HEX)
	}

	return starlark.Bytes(data), nil
}

// _FromInt is hex digits left-padded with zeros to the width given, and not
// truncated when the number is wider.
//
// The width counts digits, as binary's from_int counts its own. It is charged
// to the run before the digits are made: a script names it, and Go refuses an
// allocation it cannot make by ending the process rather than the script.
//
// The zeros are added here rather than by a width given to fmt, which refuses
// a width over a million and writes its refusal into the text it returns: the
// call would succeed with the wrong digits.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's int2hex, whose width counted
//     bits
//   - 2026-10-08 17:47: a member of the hex module, as from_int, its width counting
//     digits
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

	digits := strings.ToUpper(number.Text(BASE))
	if len(digits) < width {
		digits = strings.Repeat(PAD, width-len(digits)) + digits
	}

	return starlark.String(digits), nil
}

// _ToInt reads hex as an unsigned integer, an optional 0x taken off first.
//
// Empty text is refused rather than read as zero, because it states no number.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's hex2int
//   - 2026-10-08 17:47: a member of the hex module, as to_int, reading the digits as
//     bytes rather than through bits, and refusing empty text as not hex
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

	data, err := _Decode(fn.Name(), text)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("%s got %q: %w", fn.Name(), text, ERR_HEX)
	}

	return starlark.MakeBigInt(new(big.Int).SetBytes(data)), nil
}

// _FromBinary is bits as hex, an optional 0b taken off first, the bits
// left-padded to whole digits so the last digit is whole.
//
// The bits are decoded a byte at a time, which pads them to whole bytes. That
// is one digit more than whole digits when their count is odd, and the extra is
// a zero digit in front, which is dropped. The prefix is taken off before the
// bits are counted, or its two characters would count as bits.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bits2hex
//   - 2026-10-08 17:47: a member of the hex module, as from_binary, doing what
//     codec's _HexOfBits did with the bits decoded by binary
//   - 2026-10-08 18:32: takes an optional 0b off first, as binary's readers do
func _FromBinary(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	bits := unpack.Unprefixed(text, binary.PREFIX)

	data, err := binary.Decode(fn.Name(), bits)
	if err != nil {
		return nil, err
	}

	// A digit for every four bits, the last perhaps short of four.
	digits := (len(bits) + BITS_PER_DIGIT - 1) / BITS_PER_DIGIT
	whole := fmt.Sprintf("%X", data)

	return starlark.String(whole[len(whole)-digits:]), nil
}

// _ToBinary is hex as bits, an optional 0x taken off first, left-padded to
// whole bytes so it reads as bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's hex2bits
//   - 2026-10-08 17:47: a member of the hex module, as to_binary, writing the bits
//     through binary
func _ToBinary(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	data, err := _Decode(fn.Name(), text)
	if err != nil {
		return nil, err
	}

	return starlark.String(binary.Encode(data)), nil
}

// _Decode is hex digits as bytes, an optional 0x in either case taken off first
// and an odd count padded with a zero digit in front.
//
// The digits are padded to whole bytes rather than refused, and that is the
// same rule as padding bits to a byte: one zero digit is four zero bits.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's _BitsOfHex
//   - 2026-10-03 17:00: decodes the digits with encoding/hex rather than a big.Int for
//     each one
//   - 2026-10-08 17:47: returns the bytes, which to_int reads as well as to_binary,
//     and takes the prefix off once in either case
func _Decode(who string, text string) ([]byte, error) {
	digits := unpack.Unprefixed(text, PREFIX)

	if len(digits)%DIGITS_PER_BYTE != 0 {
		digits = PAD + digits
	}

	data, err := hex.DecodeString(digits)
	if err != nil {
		return nil, fmt.Errorf("%s got %q: %w", who, text, ERR_HEX)
	}

	return data, nil
}
