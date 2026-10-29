package unpack_test

import (
	"bytes"
	"errors"
	"testing"
	"unsafe"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// WHO is the name a refusal gives, and TEXT the data each view reads.
	WHO  = "probe"
	TEXT = "Hello, World"
)

// TestView_SharesTheValue reads a string and bytes, and makes sure that each
// view holds the bytes of the value at the address of the value. The same
// address is what shows that no copy was made.
//
// Revisions:
//   - 2026-10-29 01:08: initial creation
func TestView_SharesTheValue(t *testing.T) {
	cases := []struct {
		name  string
		given starlark.Value
	}{
		{
			"string",
			starlark.String(TEXT),
		},
		{
			"bytes",
			starlark.Bytes(TEXT),
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				view, err := unpack.View(
					WHO,
					item.given,
				)
				if err != nil {
					t.Fatalf(
						"view: %v",
						err,
					)
				}

				if !bytes.Equal(
					view,
					[]byte(TEXT),
				) {
					t.Fatalf(
						"view is %q, want %q",
						view,
						TEXT,
					)
				}

				if unsafe.SliceData(view) != _Address(item.given) {
					t.Fatalf("view is a copy, not the value")
				}
			},
		)
	}
}

// TestView_RefusesAsBytes gives View and DataView what they cannot read. Each
// refusal is the refusal of Bytes or Data, with the same text, so the second
// contract refuses what the first refuses.
//
// Revisions:
//   - 2026-10-29 01:08: initial creation
func TestView_RefusesAsBytes(t *testing.T) {
	var (
		fn = starlark.NewBuiltin(
			WHO,
			nil,
		)
		cases = []struct {
			name string
			args starlark.Tuple
		}{
			{
				"an int",
				starlark.Tuple{
					starlark.MakeInt(1),
				},
			},
			{
				"none",
				starlark.Tuple{
					starlark.None,
				},
			},
			{
				"no argument",
				starlark.Tuple{},
			},
			{
				"two arguments",
				starlark.Tuple{
					starlark.String(TEXT),
					starlark.String(TEXT),
				},
			},
		}
	)

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					_, viewed = unpack.DataView(
						fn,
						item.args,
						nil,
					)
					_, copied = unpack.Data(
						fn,
						item.args,
						nil,
					)
				)

				_Same(
					t,
					viewed,
					copied,
				)
			},
		)
	}
}

// _Same fails unless two refusals are both there and have the same text, and
// the view refusal is ErrData when the copy refusal is.
//
// Revisions:
//   - 2026-10-29 01:08: initial creation
func _Same(
	t *testing.T,
	viewed error,
	copied error,
) {
	t.Helper()

	if viewed == nil || copied == nil {
		t.Fatalf(
			"view refused with %v, copy with %v; want both to refuse",
			viewed,
			copied,
		)
	}

	if viewed.Error() != copied.Error() {
		t.Fatalf(
			"view refused with %q, want %q",
			viewed,
			copied,
		)
	}

	var (
		want = errors.Is(
			copied,
			unpack.ErrData,
		)
		got = errors.Is(
			viewed,
			unpack.ErrData,
		)
	)

	if got != want {
		t.Fatalf(
			"view refusal is ErrData: %t, want %t",
			got,
			want,
		)
	}
}

// _Address is where the bytes of a string or bytes value start.
//
// Revisions:
//   - 2026-10-29 01:08: initial creation
func _Address(given starlark.Value) *byte {
	switch held := given.(type) {
	case starlark.Bytes:
		return unsafe.StringData(string(held))

	case starlark.String:
		return unsafe.StringData(string(held))
	}

	return nil
}
