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
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package binary

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/bits"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	// ErrBits is bits.ErrBits.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrBits = bits.ErrBits
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
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Binary struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func (b *_Binary) Name() string {
	return spec.NAME
}

// Values returns the binary module.
//
// Revisions:
//   - 2026-10-08 17:47: initial creation
//   - 2026-11-08 00:59: names from spec, helpers as shared (C91w12c, Q228)
//   - 2026-11-10 11:32: uses bits.ToBytes and bits.ToInt, which moved to internal/bits (C120b,
//     Q315)
func (b *_Binary) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.FROM_BYTES: starlark.NewBuiltin(
					spec.NAME+"."+spec.FROM_BYTES,
					_FromBytes,
				),
				spec.TO_BYTES: starlark.NewBuiltin(
					spec.NAME+"."+spec.TO_BYTES,
					bits.ToBytes,
				),
				spec.FROM_INT: starlark.NewBuiltin(
					spec.NAME+"."+spec.FROM_INT,
					shared.FromInt(spec.BASE),
				),
				spec.TO_INT: starlark.NewBuiltin(
					spec.NAME+"."+spec.TO_INT,
					bits.ToInt,
				),
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
//   - 2026-11-03 23:00: reads a view of the data, with no copy: Encode only reads it
//     and keeps none of it (C38b, plan D4)
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
