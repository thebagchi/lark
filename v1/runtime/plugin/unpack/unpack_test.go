package unpack_test

import (
	"math/big"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Reader is one of the helpers that read a whole call, reduced to its error.
type _Reader func(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error

// TestHelpers_RefuseAsUnpackPositionalArgsDoes calls Text, Data, DataView and
// Counted with every wrong shape of a call, beside a reader that unpacks the
// same call with starlark.UnpackPositionalArgs and then checks what the
// helper checks. Each refusal must have the text of that reader, and a call it
// accepts must be accepted.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func TestHelpers_RefuseAsUnpackPositionalArgsDoes(t *testing.T) {
	var (
		fn = starlark.NewBuiltin(
			WHO,
			nil,
		)

		keyword = []starlark.Tuple{
			{
				starlark.String("data"),
				starlark.String("k"),
			},
		}

		huge = starlark.MakeBigInt(new(big.Int).Lsh(
			big.NewInt(1),
			70,
		))

		single = map[string]struct {
			args   starlark.Tuple
			kwargs []starlark.Tuple
		}{
			"nothing": {},
			"two": {
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.String("k"),
				},
			},
			"a keyword alone": {
				kwargs: keyword,
			},
			"a keyword beside one": {
				args: starlark.Tuple{
					starlark.String("k"),
				},
				kwargs: keyword,
			},
			"an int": {
				args: starlark.Tuple{
					starlark.MakeInt(1),
				},
			},
			"None": {
				args: starlark.Tuple{
					starlark.None,
				},
			},
			"a string": {
				args: starlark.Tuple{
					starlark.String("k"),
				},
			},
			"bytes": {
				args: starlark.Tuple{
					starlark.Bytes("k"),
				},
			},
		}

		counted = map[string]struct {
			args   starlark.Tuple
			kwargs []starlark.Tuple
		}{
			"nothing": {},
			"one": {
				args: starlark.Tuple{
					starlark.MakeInt(1),
				},
			},
			"three": {
				args: starlark.Tuple{
					starlark.MakeInt(1),
					starlark.MakeInt(1),
					starlark.MakeInt(1),
				},
			},
			"a keyword beside two": {
				args: starlark.Tuple{
					starlark.MakeInt(1),
					starlark.MakeInt(1),
				},
				kwargs: keyword,
			},
			"a number that is a string": {
				args: starlark.Tuple{
					starlark.String("1"),
					starlark.MakeInt(1),
				},
			},
			"a width that is a string": {
				args: starlark.Tuple{
					starlark.MakeInt(1),
					starlark.String("1"),
				},
			},
			"both None": {
				args: starlark.Tuple{
					starlark.None,
					starlark.None,
				},
			},
			"a negative number": {
				args: starlark.Tuple{
					starlark.MakeInt(-1),
					starlark.MakeInt(1),
				},
			},
			"a width of zero": {
				args: starlark.Tuple{
					starlark.MakeInt(1),
					starlark.MakeInt(0),
				},
			},
			"a width past an int": {
				args: starlark.Tuple{
					starlark.MakeInt(1),
					huge,
				},
			},
			"a number past an int": {
				args: starlark.Tuple{
					huge,
					starlark.MakeInt(1),
				},
			},
			"a number and a width": {
				args: starlark.Tuple{
					starlark.MakeInt(255),
					starlark.MakeInt(2),
				},
			},
		}

		readers = map[string]struct {
			got  _Reader
			want _Reader
		}{
			"Text": {
				got:  _Text,
				want: _UnpackedText,
			},
			"Data": {
				got:  _Data,
				want: _UnpackedData,
			},
			"DataView": {
				got:  _DataView,
				want: _UnpackedData,
			},
		}
	)

	for reader, pair := range readers {
		for name, tc := range single {
			_Refuses(
				t,
				reader+"/"+name,
				func() (error, error) {
					return pair.got(
							fn,
							tc.args,
							tc.kwargs,
						), pair.want(
							fn,
							tc.args,
							tc.kwargs,
						)
				},
			)
		}
	}

	for name, tc := range counted {
		_Refuses(
			t,
			"Counted/"+name,
			func() (error, error) {
				return _Counted(
						fn,
						tc.args,
						tc.kwargs,
					), _UnpackedCounted(
						fn,
						tc.args,
						tc.kwargs,
					)
			},
		)
	}
}

// TestRegression_HelpersAllocateOnlyTheirResult counts what each helper
// allocates for a call it accepts.
//
// Through starlark.UnpackPositionalArgs, each local a helper unpacked into
// went to the heap: one allocation for each argument, on every call. What is
// left is what the helper gives back: Data copies the input, and Counted
// gives a big.Int.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func TestRegression_HelpersAllocateOnlyTheirResult(t *testing.T) {
	var (
		fn = starlark.NewBuiltin(
			WHO,
			nil,
		)

		data = starlark.Tuple{
			starlark.String(TEXT),
		}

		count = starlark.Tuple{
			starlark.MakeInt(255),
			starlark.MakeInt(2),
		}

		cases = map[string]struct {
			read _Reader
			args starlark.Tuple
			want float64
		}{
			"Text": {
				read: _Text,
				args: data,
				want: 0,
			},
			"DataView": {
				read: _DataView,
				args: data,
				want: 0,
			},
			"Data": {
				read: _Data,
				args: data,
				want: 1,
			},
			"Counted": {
				read: _Counted,
				args: count,
				want: 2,
			},
		}
	)

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				got := testing.AllocsPerRun(
					100,
					func() {
						err := tc.read(
							fn,
							tc.args,
							nil,
						)
						if err != nil {
							t.Fatal(err)
						}
					},
				)

				if got != tc.want {
					t.Fatalf(
						"got %v allocations, want %v",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// _Refuses runs one case of a helper beside its reader, and fails when the
// two disagree on whether the call is refused, or on the text of a refusal.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func _Refuses(
	t *testing.T,
	name string,
	both func() (error, error),
) {
	t.Helper()

	t.Run(
		name,
		func(t *testing.T) {
			got, want := both()

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

// _Text is unpack.Text, reduced to its error.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func _Text(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	_, err := unpack.Text(
		fn,
		args,
		kwargs,
	)

	return err
}

// _Data is unpack.Data, reduced to its error.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func _Data(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	_, err := unpack.Data(
		fn,
		args,
		kwargs,
	)

	return err
}

// _DataView is unpack.DataView, reduced to its error.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func _DataView(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	_, err := unpack.DataView(
		fn,
		args,
		kwargs,
	)

	return err
}

// _Counted is unpack.Counted, reduced to its error.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func _Counted(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	_, _, err := unpack.Counted(
		fn,
		args,
		kwargs,
	)

	return err
}

// _UnpackedText reads one string with starlark.UnpackPositionalArgs.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func _UnpackedText(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	var text string

	return starlark.UnpackPositionalArgs(
		fn.Name(),
		args,
		kwargs,
		1,
		&text,
	)
}

// _UnpackedData reads one value with starlark.UnpackPositionalArgs, and then
// refuses it as unpack.Bytes does.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func _UnpackedData(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	var given starlark.Value

	err := starlark.UnpackPositionalArgs(
		fn.Name(),
		args,
		kwargs,
		1,
		&given,
	)
	if err != nil {
		return err
	}

	_, err = unpack.Bytes(
		fn.Name(),
		given,
	)

	return err
}

// _UnpackedCounted reads an int and a width with
// starlark.UnpackPositionalArgs, and then refuses them as unpack.Counted
// does: the first of a negative number and a width below one.
//
// Revisions:
//   - 2026-10-29 23:55: initial creation
func _UnpackedCounted(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	var (
		number starlark.Int
		width  int
	)

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		fn.Name(),
		args,
		kwargs,
		NEEDED,
		&number,
		&width,
	)
	if err != nil {
		return err
	}

	_, _, err = unpack.Counted(
		fn,
		starlark.Tuple{
			number,
			starlark.MakeInt(width),
		},
		nil,
	)

	return err
}
