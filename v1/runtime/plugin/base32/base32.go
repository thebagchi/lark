// Package base32 gives a script RFC 4648 base32. Importing it is what enables
// it.
//
// A module, for the reason base64 is one: encode and decode are words a script
// uses for several things, and they read better behind the encoding they
// belong to.
//
// Here at all because it is what a one-time password secret and some DNS
// records are written in, and it is one call over the standard library.
package base32

import (
	"encoding/base32"
	"errors"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrEncoded is returned for text that is not base32.
var ErrEncoded = errors.New("not the encoding this reads")

const (
	// NAME is the module, and the two names it holds.
	NAME   = "base32"
	ENCODE = "encode"
	DECODE = "decode"

	// PADDING is what the padded alphabet ends with, and what the decoder
	// takes off before reading.
	PADDING = "="
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
type _Base32 struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (b *_Base32) Name() string {
	return NAME
}

// Values returns the base32 module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (b *_Base32) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				ENCODE: starlark.NewBuiltin(NAME+"."+ENCODE, _Encode),
				DECODE: starlark.NewBuiltin(NAME+"."+DECODE, _Decode),
			},
		},
	}
}

// _Encode is RFC 4648 base32, padded.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b32encode
//   - 2026-09-24 00:53: a member of the base32 module
func _Encode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(base32.StdEncoding.EncodeToString(data)), nil
}

// _Decode reads base32, padded or not.
//
// Padding off rather than required, for the reason base64's reader gives: both
// spellings of one value are in the wild, and a decoder that accepted only its
// own output would refuse half of what a script is handed.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b32decode
//   - 2026-09-24 00:53: a member of the base32 module
func _Decode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	bare := base32.StdEncoding.WithPadding(base32.NoPadding)

	data, err := bare.DecodeString(strings.TrimRight(text, PADDING))
	if err != nil {
		return nil, fmt.Errorf("%s got %q: %w", fn.Name(), text, ErrEncoded)
	}

	return starlark.Bytes(data), nil
}
