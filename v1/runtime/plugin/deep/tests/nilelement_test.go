package tests

// This file tests B293: a nil value held in a container is not data. A script
// cannot build one; a Go host plugin can, so the tests hand one to a script
// through a plugin and run event.post, state.set and q.push on it (C131, Q429).
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293, Q429)

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _NilHost is a plugin of a host that hands a script containers which hold a
// nil value. Named hostnil.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293)
type _NilHost struct{}

// Name is hostnil.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293)
func (h *_NilHost) Name() string {
	return "hostnil"
}

// Values is the module hostnil: list, tuple, dict and nested, each holding a nil.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293)
func (h *_NilHost) Values() starlark.StringDict {
	return starlark.StringDict{
		"hostnil": &starlarkstruct.Module{
			Name: "hostnil",
			Members: starlark.StringDict{
				"list": _Giving(func() starlark.Value {
					return _Holding(nil)
				}),
				"tuple": _Giving(func() starlark.Value {
					return starlark.Tuple{starlark.MakeInt(1), nil}
				}),
				"dict": _Giving(func() starlark.Value {
					dict := starlark.NewDict(1)

					_ = dict.SetKey(
						starlark.String("k"),
						nil,
					)

					return dict
				}),
				"nested": _Giving(func() starlark.Value {
					return _Holding(
						starlark.MakeInt(1),
						starlark.Tuple{_Holding(nil)},
					)
				}),
			},
		},
	}
}

// _Holding is a list of elements, which may be nil.
//
// Revisions:
//   - 2026-11-13 09:43: initial creation (C131, B293)
func _Holding(elements ...starlark.Value) *starlark.List {
	return starlark.NewList(elements)
}

// _Giving is a builtin that answers what build builds.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293)
func _Giving(build func() starlark.Value) *starlark.Builtin {
	return starlark.NewBuiltin(
		"give",
		func(
			_ *starlark.Thread,
			_ *starlark.Builtin,
			_ starlark.Tuple,
			_ []starlark.Tuple,
		) (starlark.Value, error) {
			return build(), nil
		},
	)
}

// TestIsData_ANilIsNotData proves IsData answers no for a nil value, alone or
// held at any depth, and names it nil. The walk does not panic and returns no
// error.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293, Q429)
func TestIsData_ANilIsNotData(t *testing.T) {
	dict := starlark.NewDict(1)

	_ = dict.SetKey(
		starlark.String("k"),
		nil,
	)

	tests := []struct {
		name  string
		value starlark.Value
	}{
		{
			"the value itself",
			nil,
		},
		{
			"a list",
			_Holding(nil),
		},
		{
			"after good elements",
			_Holding(
				starlark.MakeInt(1),
				starlark.None,
				nil,
			),
		},
		{
			"a tuple",
			starlark.Tuple{nil},
		},
		{
			"a dict value",
			dict,
		},
		{
			"nested",
			_Holding(starlark.Tuple{_Holding(nil)}),
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				bad, ok, err := deep.IsData(
					context.Background(),
					test.value,
				)
				if err != nil {
					t.Fatal(err)
				}

				if ok {
					t.Fatal("a nil was taken for data")
				}

				if bad == nil || bad.Type() != "nil" {
					t.Fatalf(
						"bad is %v, want a value of type nil",
						bad,
					)
				}
			},
		)
	}
}

// TestMissing_IsAValueThatIsNotThere proves the value Walk hands back for a nil
// answers every question of a Starlark value without a panic: its text, its
// type, its truth, and that it cannot be frozen into anything or be a key.
//
// Revisions:
//   - 2026-11-13 10:01: initial creation (C131, B293, Q429)
func TestMissing_IsAValueThatIsNotThere(t *testing.T) {
	bad, _, _ := deep.IsData(
		context.Background(),
		nil,
	)

	bad.Freeze()

	if bad.String() != "nil" || bad.Type() != "nil" || bad.Truth() != starlark.False {
		t.Fatalf(
			"got %q of type %q, truth %v",
			bad.String(),
			bad.Type(),
			bad.Truth(),
		)
	}

	_, err := bad.Hash()
	if err == nil {
		t.Fatal("a missing value hashed")
	}
}

// TestNilElement_IsRefusedWithConvert proves event.post, state.set and q.push
// each end the run with unpack.ErrConvert for a container holding a nil, and do
// not end it with a recovered panic (B293).
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293, Q429)
func TestNilElement_IsRefusedWithConvert(t *testing.T) {
	var calls = []struct{ name, call string }{
		{
			"event.post",
			`event.post("k", %s)`,
		},
		{
			"state.set",
			`state.set("k", %s)`,
		},
		{
			"q.push",
			`queue.new().push(%s)`,
		},
	}

	var shapes = []string{
		"hostnil.list()",
		"hostnil.tuple()",
		"hostnil.dict()",
		"hostnil.nested()",
	}

	for _, call := range calls {
		for _, shape := range shapes {
			t.Run(
				call.name+" "+shape,
				func(t *testing.T) {
					err := _Ran(
						t,
						"def main():\n    "+_Filled(
							call.call,
							shape,
						)+"\n",
					)
					if !errors.Is(
						err,
						unpack.ErrConvert,
					) {
						t.Fatalf(
							"got %v, want ErrConvert",
							err,
						)
					}
				},
			)
		}
	}
}

// _Filled puts shape in the one %s of format.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293)
func _Filled(
	format string,
	shape string,
) string {
	for index := range format {
		if format[index] == '%' {
			return format[:index] + shape + format[index+2:]
		}
	}

	return format
}

// _Ran runs src with the hostnil plugin and answers the error of the run.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, B293)
func _Ran(
	t *testing.T,
	src string,
) error {
	t.Helper()

	built, err := runtime.Compile(
		&common.Source{
			Entry: "nil.star",
			Text:  []byte(src),
		},
		runtime.WithPlugins(new(_NilHost)),
	)
	if err != nil {
		return err
	}

	answered := make(
		chan error,
		1,
	)

	go func() {
		run, err := runtime.Start(
			context.Background(),
			built,
		)
		if err != nil {
			answered <- err

			return
		}

		_, err = run.Wait()
		answered <- err
	}()

	select {
	case err := <-answered:
		return err

	case <-time.After(20 * time.Second):
		t.Fatal("the script did not finish")
	}

	return nil
}
