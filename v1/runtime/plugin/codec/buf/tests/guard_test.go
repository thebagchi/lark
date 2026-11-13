package tests

import (
	"slices"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
)

// TestMethods_RefuseWhatBreaksUnpack proves a method of a Buffer or a Reader,
// taken through Attr and called from Go with a nil Value or a malformed keyword
// pair, returns an error and does not panic.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestMethods_RefuseWhatBreaksUnpack(t *testing.T) {
	var (
		one   = starlark.MakeInt(1)
		name  = starlark.String("n")
		shape = []struct {
			name   string
			args   starlark.Tuple
			kwargs []starlark.Tuple
		}{
			{
				name: "nil argument",
				args: starlark.Tuple{nil},
			},
			{
				name: "nil after a good one",
				args: starlark.Tuple{one, nil},
			},
			{
				name:   "pair of one",
				kwargs: []starlark.Tuple{{name}},
			},
			{
				name:   "pair of none",
				kwargs: []starlark.Tuple{{}},
			},
			{
				name:   "name is not a string",
				kwargs: []starlark.Tuple{{one, one}},
			},
			{
				name:   "value is nil",
				kwargs: []starlark.Tuple{{name, nil}},
			},
		}
	)

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env["buf"].(starlark.HasAttrs)
	if !ok {
		t.Fatal("no buf module")
	}

	for _, made := range []struct {
		name string
		call starlark.Tuple
		fn   string
	}{
		{
			name: "buffer",
			fn:   "new",
		},
		{
			name: "reader",
			fn:   "reader",
			call: starlark.Tuple{starlark.Bytes("abcd")},
		},
	} {
		constructor, err := module.Attr(made.fn)
		if err != nil {
			t.Fatalf(
				"attr %s: %v",
				made.fn,
				err,
			)
		}

		value, err := starlark.Call(
			new(starlark.Thread),
			constructor,
			made.call,
			nil,
		)
		if err != nil {
			t.Fatalf(
				"%s: %v",
				made.fn,
				err,
			)
		}

		held, ok := value.(starlark.HasAttrs)
		if !ok {
			t.Fatalf(
				"%s has no attributes",
				made.name,
			)
		}

		names := held.AttrNames()
		if len(names) == 0 {
			t.Fatalf(
				"%s has no methods",
				made.name,
			)
		}

		for _, method := range slices.Sorted(slices.Values(names)) {
			attr, err := held.Attr(method)
			if err != nil || attr == nil {
				t.Fatalf(
					"attr %s: %v",
					method,
					err,
				)
			}

			for _, bad := range shape {
				t.Run(
					made.name+"/"+method+"/"+bad.name,
					func(t *testing.T) {
						var failure error

						raised := guard.Contained(func() {
							_, failure = starlark.Call(
								new(starlark.Thread),
								attr,
								bad.args,
								bad.kwargs,
							)
						})
						if raised != nil {
							t.Fatalf(
								"%s panicked: %v",
								method,
								raised,
							)
						}

						if failure == nil {
							t.Fatalf(
								"%s returned no error",
								method,
							)
						}
					},
				)
			}
		}
	}
}
