// Package field is internal, not public: it holds the integer module and the
// conversions between an integer and its bytes.
//
// Package field is the behavior behind package integer: the module integer, and
// the conversions between an integer and its bytes.
//
// Package integer forwards to it. The data the conversions take, common.Field,
// and the byte orders, common.BIG and common.LITTLE, are in package common.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:16: moved here from integer (C91w8b)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
package field

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// NAME is the module, and the two names it holds.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	NAME       = "integer"
	FROM_BYTES = "from_bytes"
	TO_BYTES   = "to_bytes"

	// N, WIDTH, DATA, ORDER and SIGNED are what the two functions call their
	// arguments, for a script that names one, and OPTIONAL marks an argument a
	// script may leave out.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	N        = "n"
	WIDTH    = "width"
	DATA     = "data"
	ORDER    = "order"
	SIGNED   = "signed"
	OPTIONAL = "?"
)

var (
	// DATA_NAMES are the parameters of from_bytes, in order, the first
	// required. A name that ends with OPTIONAL is one a call may leave out.
	//
	// Revisions:
	//   - 2026-11-04 06:52: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	DATA_NAMES = [...]string{
		DATA,
		ORDER + OPTIONAL,
		SIGNED + OPTIONAL,
	}

	// BYTE_NAMES are the parameters of to_bytes, in order, the first two
	// required.
	//
	// Revisions:
	//   - 2026-11-04 06:52: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	BYTE_NAMES = [...]string{
		N,
		WIDTH,
		ORDER + OPTIONAL,
		SIGNED + OPTIONAL,
	}
)

// New returns the integer plugin, a common.Plugin. Package integer registers it from its init,
// so that a host importing that package for its side effect is the whole of
// enabling it.
//
// Revisions:
//   - 2026-11-07 19:16: initial creation, in place of the init that registered it (C91w8b)
func New() *Plugin {
	return new(Plugin)
}

// Plugin is the integer plugin. Empty: every conversion works on what it is given.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
type Plugin struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-10-08 18:42: initial creation
func (i *Plugin) Name() string {
	return NAME
}

// Values returns the integer module.
//
// Revisions:
//   - 2026-10-08 18:42: initial creation
//   - 2026-11-10 01:00: now uses FromBytes, ToBytes, were _FromBytes, _ToBytes (C120a, Q314)
func (i *Plugin) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				FROM_BYTES: starlark.NewBuiltin(
					NAME+"."+FROM_BYTES,
					FromBytes,
				),
				TO_BYTES: starlark.NewBuiltin(
					NAME+"."+TO_BYTES,
					ToBytes,
				),
			},
		},
	}
}

// FromBytes is data as an integer: big-endian unless the order is "little",
// and unsigned unless signed is True.
//
// The data is read through a view, with no copy: Decode reads it once and keeps
// nothing of it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bytes2int
//   - 2026-10-08 18:42: a member of the integer module, as from_bytes
//   - 2026-10-08 21:08: takes a byte order, and reads through Decode
//   - 2026-10-08 21:37: takes signed, and lets a script name any argument
//   - 2026-11-04 00:45: reads the arguments with no local on the heap, views the
//     data, and returns what Decode gives as it is (C38c)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
//   - 2026-11-10 00:25: renamed from _FromBytes, as a test in tests/ needs it (C120a, Q314)
func FromBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	given, order, signed, err := _DataArgs(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	data, err := unpack.View(
		shared.Called(fn),
		given,
	)
	if err != nil {
		return nil, err
	}

	decode := Decode
	if signed {
		decode = DecodeSigned
	}

	return decode(
		shared.Called(fn),
		data,
		order,
	)
}

// _DataArgs reads the data, the order, and whether it is signed.
//
// A regular call, with every argument of the type it wants, is read with
// unpack.Named, which allocates nothing. Any other call goes to
// starlark.UnpackArgs, so the refusal is its text.
//
// Revisions:
//   - 2026-11-04 00:45: initial creation, out of _FromBytes (C38c)
//   - 2026-11-10 01:00: now uses DataArgsRefused, were _DataArgsRefused (C120a, Q314)
func _DataArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, string, bool, error) {
	var slots [len(DATA_NAMES)]starlark.Value

	if unpack.Named(
		args,
		kwargs,
		DATA_NAMES[:],
		slots[:],
	) {
		var (
			order, okOrder   = _OrderOf(slots[1])
			signed, okSigned = _SignedOf(slots[2])
		)

		if okOrder && okSigned {
			return slots[0], order, signed, nil
		}
	}

	return DataArgsRefused(
		fn,
		args,
		kwargs,
	)
}

// DataArgsRefused reads the data, the order, and whether it is signed, with
// starlark.UnpackArgs. Its locals go to the heap, which is why a regular call
// does not come here.
//
// Revisions:
//   - 2026-11-04 00:45: initial creation, out of _FromBytes (C38c)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
//   - 2026-11-10 00:25: renamed from _DataArgsRefused, as a test in tests/ needs it (C120a, Q314)
func DataArgsRefused(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, string, bool, error) {
	var (
		given  starlark.Value
		order  = common.BIG
		signed bool
	)

	err := starlark.UnpackArgs(
		shared.Called(fn),
		args,
		kwargs,
		DATA,
		&given,
		ORDER+OPTIONAL,
		&order,
		SIGNED+OPTIONAL,
		&signed,
	)
	if err != nil {
		return nil, "", false, err
	}

	return given, order, signed, nil
}

// _OrderOf is the order a slot holds: common.BIG for a slot the call left empty, and
// false for a value that is not a string.
//
// Revisions:
//   - 2026-11-04 00:45: initial creation (C38c)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func _OrderOf(slot starlark.Value) (string, bool) {
	if slot == nil {
		return common.BIG, true
	}

	order, ok := slot.(starlark.String)

	return string(order), ok
}

// _SignedOf is whether a slot holds True: false for a slot the call left empty,
// and false in the second result for a value that is not a bool.
//
// Revisions:
//   - 2026-11-04 00:45: initial creation (C38c)
func _SignedOf(slot starlark.Value) (bool, bool) {
	if slot == nil {
		return false, true
	}

	signed, ok := slot.(starlark.Bool)

	return bool(signed), ok
}

// ToBytes is an integer as bytes, zero-padded to the width given, big-endian
// unless the order is "little", and unsigned unless signed is True; the number
// must fit.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's int2bytes
//   - 2026-10-08 17:47: refuses with unpack's ErrRange, which hex and binary refuse
//     with too
//   - 2026-10-08 18:42: a member of the integer module, as to_bytes
//   - 2026-10-08 21:08: takes a byte order, and writes through Encode, which checks
//     the width
//   - 2026-10-08 21:37: takes signed, and lets a script name any argument
//   - 2026-11-01 13:00: writes through Encode without a budget
//   - 2026-11-03 00:30: hands the bytes it wrote over, with no copy of them (C37)
//   - 2026-11-04 00:50: keeps the number as a Starlark int, and calls Encode and
//     EncodeSigned by name, so the Field stays on the stack (C38c)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
//   - 2026-11-10 00:25: renamed from _ToBytes, as a test in tests/ needs it (C120a, Q314)
func ToBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		field common.Field
		data  []byte
	)

	signed, err := _ByteArgs(
		fn,
		args,
		kwargs,
		&field,
	)
	if err != nil {
		return nil, err
	}

	if signed {
		data, err = EncodeSigned(
			shared.Called(fn),
			&field,
		)
	} else {
		data, err = Encode(
			shared.Called(fn),
			&field,
		)
	}

	if err != nil {
		return nil, err
	}

	return starlark.Bytes(unpack.Handover(data)), nil
}

// _ByteArgs reads the number, the width and the order into field, and gives
// whether the number is signed. The caller owns field, so a regular call makes
// no Field on the heap.
//
// A regular call, with every argument of the type it wants, is read with
// unpack.Named, which allocates nothing. Any other call goes to
// starlark.UnpackArgs, so the refusal is its text.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-04 00:50: reads a regular call with no local on the heap, into
//     the Field of the caller (C38c)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
//   - 2026-11-10 01:00: now uses ByteArgsRefused, were _ByteArgsRefused (C120a, Q314)
func _ByteArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	field *common.Field,
) (bool, error) {
	var slots [len(BYTE_NAMES)]starlark.Value

	if unpack.Named(
		args,
		kwargs,
		BYTE_NAMES[:],
		slots[:],
	) {
		var (
			number, okNumber = slots[0].(starlark.Int)
			width, okWidth   = _WidthOf(slots[1])
			order, okOrder   = _OrderOf(slots[2])
			signed, okSigned = _SignedOf(slots[3])
		)

		if okNumber && okWidth && okOrder && okSigned {
			field.Number = number
			field.Width = width
			field.Order = order

			return signed, nil
		}
	}

	return ByteArgsRefused(
		fn,
		args,
		kwargs,
		field,
	)
}

// ByteArgsRefused reads the number, the width, the order, and whether it is
// signed, with starlark.UnpackArgs. Its locals go to the heap, which is why a
// regular call does not come here.
//
// Revisions:
//   - 2026-11-04 00:50: initial creation, out of _ByteArgs (C38c)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
//   - 2026-11-10 00:25: renamed from _ByteArgsRefused, as a test in tests/ needs it (C120a, Q314)
func ByteArgsRefused(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	field *common.Field,
) (bool, error) {
	var (
		number starlark.Int
		width  int
		order  = common.BIG
		signed bool
	)

	err := starlark.UnpackArgs(
		shared.Called(fn),
		args,
		kwargs,
		N,
		&number,
		WIDTH,
		&width,
		ORDER+OPTIONAL,
		&order,
		SIGNED+OPTIONAL,
		&signed,
	)
	if err != nil {
		return false, err
	}

	field.Number = number
	field.Width = width
	field.Order = order

	return signed, nil
}

// _WidthOf is the width a slot holds, and false for a value that is not an int
// that fits a Go int.
//
// Revisions:
//   - 2026-11-04 00:50: initial creation (C38c)
func _WidthOf(slot starlark.Value) (int, bool) {
	number, ok := slot.(starlark.Int)
	if !ok {
		return 0, false
	}

	wide, ok := number.Int64()
	if !ok || int64(int(wide)) != wide {
		return 0, false
	}

	return int(wide), true
}
