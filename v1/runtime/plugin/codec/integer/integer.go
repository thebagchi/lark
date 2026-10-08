// Package integer gives a script the module integer: an integer converted to
// and from its bytes. Importing it is what enables it.
//
// integer rather than int or bytes, because Starlark already gives those names
// to its own builtins, and a module of either name would hide one. The two
// functions read as Python's int.from_bytes and int.to_bytes, and take what
// those take: a byte order, big unless "little" is named, and signed = True for
// two's complement, unsigned otherwise. Any argument may be named.
//
// Encode, EncodeSigned, Decode and DecodeSigned are the conversions themselves,
// exported so buf writes and reads integers by these rules rather than by a
// second copy of them.
package integer

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// NAME is the module, and the two names it holds.
	NAME       = "integer"
	FROM_BYTES = "from_bytes"
	TO_BYTES   = "to_bytes"

	// N, WIDTH, DATA, ORDER and SIGNED are what the two functions call their
	// arguments, for a script that names one, and OPTIONAL marks an argument a
	// script may leave out.
	N        = "n"
	WIDTH    = "width"
	DATA     = "data"
	ORDER    = "order"
	SIGNED   = "signed"
	OPTIONAL = "?"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-10-08 18:42: initial creation
func init() {
	plugin.Register(new(_Integer))
}

// _Integer is the plugin. Empty: every conversion works on what it is given.
type _Integer struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-10-08 18:42: initial creation
func (i *_Integer) Name() string {
	return NAME
}

// Values returns the integer module.
//
// Revisions:
//   - 2026-10-08 18:42: initial creation
func (i *_Integer) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				FROM_BYTES: starlark.NewBuiltin(NAME+"."+FROM_BYTES, _FromBytes),
				TO_BYTES:   starlark.NewBuiltin(NAME+"."+TO_BYTES, _ToBytes),
			},
		},
	}
}

// _FromBytes is data as an integer: big-endian unless the order is "little",
// and unsigned unless signed is True.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bytes2int
//   - 2026-10-08 18:42: a member of the integer module, as from_bytes
//   - 2026-10-08 21:08: takes a byte order, and reads through Decode
//   - 2026-10-08 21:37: takes signed, and lets a script name any argument
func _FromBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		given  starlark.Value
		order  = BIG
		signed bool
	)

	err := starlark.UnpackArgs(
		fn.Name(),
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
		return nil, err
	}

	data, err := unpack.Bytes(fn.Name(), given)
	if err != nil {
		return nil, err
	}

	decode := Decode
	if signed {
		decode = DecodeSigned
	}

	number, err := decode(fn.Name(), data, order)
	if err != nil {
		return nil, err
	}

	return starlark.MakeBigInt(number), nil
}

// _ToBytes is an integer as bytes, zero-padded to the width given, big-endian
// unless the order is "little", and unsigned unless signed is True; the number
// must fit.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's int2bytes
//   - 2026-10-08 17:47: refuses with unpack's ERR_RANGE, which hex and binary refuse
//     with too
//   - 2026-10-08 18:42: a member of the integer module, as to_bytes
//   - 2026-10-08 21:08: takes a byte order, and writes through Encode, which charges
//     the width to the run
//   - 2026-10-08 21:37: takes signed, and lets a script name any argument
func _ToBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		number starlark.Int
		width  int
		order  = BIG
		signed bool
	)

	err := starlark.UnpackArgs(
		fn.Name(),
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
		return nil, err
	}

	encode := Encode
	if signed {
		encode = EncodeSigned
	}

	field := &Field{Number: number.BigInt(), Width: width, Order: order}

	data, err := encode(scheduler.Allowance(thread), fn.Name(), field)
	if err != nil {
		return nil, err
	}

	return starlark.Bytes(data), nil
}
