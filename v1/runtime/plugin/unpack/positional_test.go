package unpack_test

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Unpacked reads a name and a change with Positional, StringAt and
// CallableAt, in the order a builtin that takes the two reads them.
//
// Revisions:
//   - 2026-10-29 01:32: initial creation
func _Unpacked(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	const NEEDED = 2

	err := unpack.Positional(
		fn,
		args,
		kwargs,
		NEEDED,
	)
	if err != nil {
		return err
	}

	_, err = unpack.StringAt(
		fn,
		args,
		0,
	)
	if err != nil {
		return err
	}

	_, err = unpack.CallableAt(
		fn,
		args,
		1,
	)

	return err
}

// TestPositional_RefusesAsUnpackPositionalArgsDoes calls Positional, StringAt
// and CallableAt, and starlark.UnpackPositionalArgs, with every wrong shape of
// a call that takes a name and a change. The helpers are a second path for the
// contract of UnpackPositionalArgs, so each refusal must have its text, and a
// call it accepts must be accepted.
//
// Revisions:
//   - 2026-10-29 01:32: initial creation
func TestPositional_RefusesAsUnpackPositionalArgsDoes(t *testing.T) {
	var (
		fn = starlark.NewBuiltin(
			WHO,
			nil,
		)

		change = starlark.NewBuiltin(
			"change",
			nil,
		)

		keyword = []starlark.Tuple{
			{
				starlark.String("name"),
				starlark.String("k"),
			},
		}

		cases = map[string]struct {
			args   starlark.Tuple
			kwargs []starlark.Tuple
		}{
			"nothing": {},
			"one": {
				args: starlark.Tuple{
					starlark.String("k"),
				},
			},
			"three": {
				args: starlark.Tuple{
					starlark.String("k"),
					change,
					change,
				},
			},
			"a keyword alone": {
				kwargs: keyword,
			},
			"a keyword beside two": {
				args: starlark.Tuple{
					starlark.String("k"),
					change,
				},
				kwargs: keyword,
			},
			"a keyword beside three": {
				args: starlark.Tuple{
					starlark.String("k"),
					change,
					change,
				},
				kwargs: keyword,
			},
			"a name that is an int": {
				args: starlark.Tuple{
					starlark.MakeInt(1),
					change,
				},
			},
			"a name that is bytes": {
				args: starlark.Tuple{
					starlark.Bytes("k"),
					change,
				},
			},
			"a name and a change that are both wrong": {
				args: starlark.Tuple{
					starlark.None,
					starlark.MakeInt(1),
				},
			},
			"a change that is a string": {
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.String("f"),
				},
			},
			"a change that is a dict": {
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.NewDict(0),
				},
			},
			"a name and a change": {
				args: starlark.Tuple{
					starlark.String("k"),
					change,
				},
			},
		}
	)

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					text     string
					callable starlark.Callable

					want = starlark.UnpackPositionalArgs(
						fn.Name(),
						tc.args,
						tc.kwargs,
						2,
						&text,
						&callable,
					)

					got = _Unpacked(
						fn,
						tc.args,
						tc.kwargs,
					)
				)

				if (got == nil) != (want == nil) {
					t.Fatalf(
						"got %v, want %v",
						got,
						want,
					)
				}

				if got != nil && got.Error() != want.Error() {
					t.Fatalf(
						"got %q, want %q",
						got.Error(),
						want.Error(),
					)
				}
			},
		)
	}
}
