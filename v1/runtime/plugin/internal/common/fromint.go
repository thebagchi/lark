package common

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// FromInt is the from_int builtin of a codec that writes digits in base: a
// non-negative integer and a width, the digits left-padded with zeros to the
// width and not truncated when the number is wider.
//
// The function it returns is given to starlark.NewBuiltin. The zeros are added
// by unpack.Padded, in one buffer, because fmt refuses a width over a million
// and writes its refusal into the text it returns.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation, from hex's and binary's _FromInt (C59, A4b)
func FromInt(base int) func(
	*starlark.Thread,
	*starlark.Builtin,
	starlark.Tuple,
	[]starlark.Tuple,
) (starlark.Value, error) {
	return func(
		_ *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		number, width, err := unpack.Counted(
			fn,
			args,
			kwargs,
		)
		if err != nil {
			return nil, err
		}

		text, err := unpack.Padded(
			number,
			base,
			width,
		)
		if err != nil {
			return nil, err
		}

		return starlark.String(text), nil
	}
}
