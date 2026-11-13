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
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package hex

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/codec"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	// ErrHex is codec.ErrHex.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrHex = codec.ErrHex
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
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Hex struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func (h *_Hex) Name() string {
	return spec.NAME
}

// Values returns the hex module.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
//   - 2026-11-08 00:59: names from spec, helpers as shared (C91w12c, Q228)
//   - 2026-11-10 11:32: uses codec.ToBytes, codec.ToInt, codec.FromBinary and codec.ToBinary,
//     which moved to internal/codec (C120b, Q315)
func (h *_Hex) Values() starlark.StringDict {
	members := starlark.StringDict{
		spec.FROM_BYTES: starlark.NewBuiltin(
			spec.NAME+"."+spec.FROM_BYTES,
			_FromBytes,
		),
		spec.TO_BYTES: starlark.NewBuiltin(
			spec.NAME+"."+spec.TO_BYTES,
			codec.ToBytes,
		),
		spec.FROM_INT: starlark.NewBuiltin(
			spec.NAME+"."+spec.FROM_INT,
			shared.FromInt(spec.BASE),
		),
		spec.TO_INT: starlark.NewBuiltin(
			spec.NAME+"."+spec.TO_INT,
			codec.ToInt,
		),
		spec.FROM_BINARY: starlark.NewBuiltin(
			spec.NAME+"."+spec.FROM_BINARY,
			codec.FromBinary,
		),
		spec.TO_BINARY: starlark.NewBuiltin(
			spec.NAME+"."+spec.TO_BINARY,
			codec.ToBinary,
		),
	}

	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name:    spec.NAME,
			Members: members,
		},
	}
}

// _FromBytes is data as hex, two digits a byte, with no separators.
//
// The data is a view of the script's value, not a copy: Encode only reads it,
// and keeps nothing of it once it returns, which is the contract of
// unpack.View.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's bytes2hex
//   - 2026-10-08 17:47: a member of the hex module, as from_bytes, writing upper case
//   - 2026-10-29 01:08: reads a view of the data rather than a copy, and writes
//     through Encode rather than fmt's %X
func _FromBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.DataView(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return starlark.String(Encode(data)), nil
}
