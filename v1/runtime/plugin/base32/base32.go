// Package base32 gives a script RFC 4648 base32. Importing it is what enables
// it.
//
// A module, for the reason base64 is one: encode and decode are words a script
// uses for several things, and they read better behind the encoding they
// belong to.
//
// Here at all because it is what a one-time password secret and some DNS
// records are written in, and it is one call over the standard library.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package base32

import (
	"encoding/base32"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/base32/internal/decode"
	"github.com/thebagchi/lark/v1/runtime/plugin/base32/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func init() {
	plugin.Register(new(_Base32))
}

// _Base32 is the plugin. Empty: every conversion works on what it is given.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Base32 struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func (b *_Base32) Name() string {
	return spec.NAME
}

// Values returns the base32 module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 11:32: uses decode.Decode, which moved to internal/decode (C120b, Q315)
func (b *_Base32) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.ENCODE: starlark.NewBuiltin(
					spec.NAME+"."+spec.ENCODE,
					_Encode,
				),
				spec.DECODE: starlark.NewBuiltin(
					spec.NAME+"."+spec.DECODE,
					decode.Decode,
				),
			},
		},
	}
}

// _Encode is RFC 4648 base32, padded.
//
// The data is a view of the script's value, not a copy: EncodeToString only
// reads it, into a buffer of its own, which is the contract of unpack.View.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b32encode
//   - 2026-09-24 00:53: a member of the base32 module
//   - 2026-10-29 23:58: reads a view of the data rather than a copy
//   - 2026-11-03 00:30: hands the output buffer over, in place of EncodeToString's copy of it
//     (C37)
func _Encode(
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

	text := make(
		[]byte,
		base32.StdEncoding.EncodedLen(len(data)),
	)
	base32.StdEncoding.Encode(
		text,
		data,
	)

	return starlark.String(unpack.Handover(text)), nil
}
