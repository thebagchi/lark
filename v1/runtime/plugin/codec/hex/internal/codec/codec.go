// Package codec holds the coding of the hex plugin: the encode and decode
// kernels with their portable, SWAR, SSE, AVX2 and AVX-512 paths, and the
// conversions between hex text, bytes, integers and bit strings that use them.
// The plugin type, its registration and its names stay in package hex.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package codec

import (
	"errors"
	"math/big"
	"unsafe"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrHex is returned for text that is not hexadecimal, or has an odd number of
// digits where whole bytes are wanted.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
var (
	ErrHex = errors.New("not hexadecimal")
)

// ToBytes is the inverse, an optional 0x taken off first; an odd number of
// digits is an error, because it is not a whole number of bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's hex2bytes
//   - 2026-10-08 17:47: a member of the hex module, as to_bytes
//   - 2026-10-08 18:06: takes an optional 0x off first, as to_int and to_binary do
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-03 01:11: decodes through _DecodeDigits rather than encoding/hex (C50)
//   - 2026-11-03 21:57: hands the decoded bytes over, with no copy of them (C37)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:59: names from spec, helpers as shared (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex and renamed _ToBytes to ToBytes,
//     for the tests in tests/ (C120b, Q315, Q314)
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

	data, ok := _DecodeDigits(unpack.Unprefixed(
		text,
		spec.PREFIX,
	))
	if !ok {
		return nil, shared.Refused(
			shared.Called(fn),
			text,
			ErrHex,
		)
	}

	return starlark.Bytes(unpack.Handover(data)), nil
}

// ToInt reads hex as an unsigned integer, an optional 0x taken off first.
//
// Empty text is refused rather than read as zero, because it states no number.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's hex2int
//   - 2026-10-08 17:47: a member of the hex module, as to_int, reading the digits as
//     bytes rather than through bits, and refusing empty text as not hex
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:59: plugin helpers named shared (C91w12c)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex and renamed _ToInt to ToInt, for
//     the tests in tests/ (C120b, Q315, Q314)
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

	data, err := _Decode(
		shared.Called(fn),
		text,
	)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, shared.Refused(
			shared.Called(fn),
			text,
			ErrHex,
		)
	}

	return starlark.MakeBigInt(new(big.Int).SetBytes(data)), nil
}

// FromBinary is bits as hex, an optional 0b taken off first, the bits
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
//   - 2026-11-02 16:55: writes the digits through Encode rather than fmt's %X, which
//     grew its buffer as it wrote: 28 allocations at 64 KiB (C31)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:59: names from spec and common, helpers as shared (C91w12c, Q228, Q244)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex and renamed _FromBinary to
//     FromBinary, for the tests in tests/ (C120b, Q315, Q314)
func FromBinary(
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

	data, err = binary.Decode(
		shared.Called(fn),
		bits,
	)
	if err != nil {
		return nil, err
	}

	// A digit for every four bits, the last perhaps short of four.
	var (
		digits = (len(bits) + spec.BITS_PER_DIGIT - 1) / spec.BITS_PER_DIGIT
		whole  = Encode(data)
	)

	return starlark.String(whole[len(whole)-digits:]), nil
}

// ToBinary is hex as bits, an optional 0x taken off first, left-padded to
// whole bytes so it reads as bytes.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's hex2bits
//   - 2026-10-08 17:47: a member of the hex module, as to_binary, writing the bits
//     through binary
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:59: plugin helpers named shared (C91w12c)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex and renamed _ToBinary to
//     ToBinary, for the tests in tests/ (C120b, Q315, Q314)
func ToBinary(
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

	data, err := _Decode(
		shared.Called(fn),
		text,
	)
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
// This reads more than RFC 4648 section 8 writes. Section 8 defines no "0x"
// prefix, and two characters to the octet. Section 3.3 lets a reader differ
// "unless the specification referring to this document explicitly states
// otherwise", and this comment and the hex module are lark's statement. The
// text here is a number as often as octets (to_int, to_binary), and the
// prefix is how scripts write one. TestHex_KeepsItsDocumentedLeniencies pins
// both.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's _BitsOfHex
//   - 2026-10-03 17:00: decodes the digits with encoding/hex rather than a big.Int for
//     each one
//   - 2026-10-08 17:47: returns the bytes, which to_int reads as well as to_binary,
//     and takes the prefix off once in either case
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 16:45: names the clause that allows the prefix and the padding (C22)
//   - 2026-11-03 01:11: decodes through _DecodeDigits rather than encoding/hex (C50)
//   - 2026-11-08 00:59: names from spec, helpers as shared (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
func _Decode(
	who string,
	text string,
) ([]byte, error) {
	digits := unpack.Unprefixed(
		text,
		spec.PREFIX,
	)

	if len(digits)%spec.DIGITS_PER_BYTE != 0 {
		digits = spec.PAD + digits
	}

	data, ok := _DecodeDigits(digits)
	if !ok {
		return nil, shared.Refused(
			who,
			text,
			ErrHex,
		)
	}

	return data, nil
}

// _DecodeDigits is the bytes that digits stand for, and whether every digit was
// one and the count was even. The error text of a caller names the whole text
// and no position, so a false says all that a caller needs.
//
// The digits are read through a view of the string, not a copy: DecodeAt only
// reads them. encoding/hex.DecodeString copied them first, and then decoded in
// place; this makes the one buffer, the result.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation, in place of encoding/hex.DecodeString (C50)
//   - 2026-11-05 19:07: decodes through _DecodeAt (C58)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
func _DecodeDigits(digits string) ([]byte, bool) {
	if len(digits)%spec.DIGITS_PER_BYTE != 0 {
		return nil, false
	}

	data := make(
		[]byte,
		len(digits)/spec.DIGITS_PER_BYTE,
	)

	return data, DecodeAt(
		cpu.LEVEL,
		data,
		unsafe.Slice(
			unsafe.StringData(digits),
			len(digits),
		),
	)
}
