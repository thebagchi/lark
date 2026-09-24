package deep_test

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/plugin/deep"
)

// _Held is a value built the way a script would build one.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
func _Held(values ...starlark.Value) *starlark.List {
	return starlark.NewList(values)
}

// TestIsData_AcceptsWhatAStoreCanHold is every shape a script builds out of
// data, including the containers nested in each other.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
func TestIsData_AcceptsWhatAStoreCanHold(t *testing.T) {
	deeper := starlark.NewDict(1)

	err := deeper.SetKey(starlark.String("k"), _Held(starlark.MakeInt(1)))
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]starlark.Value{
		"none":    starlark.None,
		"a bool":  starlark.True,
		"an int":  starlark.MakeInt(7),
		"a float": starlark.Float(1.5),
		"a word":  starlark.String("x"),
		"bytes":   starlark.Bytes("x"),
		"a list":  _Held(starlark.MakeInt(1), starlark.String("a")),
		"a tuple": starlark.Tuple{starlark.MakeInt(1)},
		"a dict":  deeper,
		"nested":  _Held(deeper, starlark.Tuple{starlark.None}),
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			bad, ok := deep.IsData(value)
			if !ok {
				t.Fatalf("%s was refused, at %v", name, bad)
			}
		})
	}
}

// TestIsData_RefusesWhatIsNotData names the thing that failed rather than the
// container it was found in, which is the whole reason the first return
// exists.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
func TestIsData_RefusesWhatIsNotData(t *testing.T) {
	code := starlark.NewBuiltin("helper", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		return starlark.None, nil
	})

	cases := map[string]starlark.Value{
		"a builtin":          code,
		"one inside a list":  _Held(starlark.MakeInt(1), code),
		"one inside a tuple": starlark.Tuple{starlark.None, code},
		"one deeper still":   _Held(_Held(_Held(code))),
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			bad, ok := deep.IsData(value)
			if ok {
				t.Fatalf("%s was accepted", name)
			}

			if bad != code {
				t.Fatalf("%s blamed %v, want the builtin itself", name, bad)
			}
		})
	}
}

// TestIsData_TerminatesOnAValueThatHoldsItself is the case the walk carries a
// seen set for. Starlark allows x = [1]; x.append(x), and a check that
// followed that reference would not come back.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
func TestIsData_TerminatesOnAValueThatHoldsItself(t *testing.T) {
	held := _Held(starlark.MakeInt(1))

	err := held.Append(held)
	if err != nil {
		t.Fatal(err)
	}

	bad, ok := deep.IsData(held)
	if !ok {
		t.Fatalf("a list holding itself was refused, at %v", bad)
	}
}
