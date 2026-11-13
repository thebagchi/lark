package tests

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestPolled_KeepsWhatAScriptSees proves the wrapped environment looks the
// same to a script: the same names, the same builtin names, the same module
// name, and values that are not builtins passed through as they are.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestPolled_KeepsWhatAScriptSees(t *testing.T) {
	var (
		inner = starlark.NewBuiltin(
			"leaf",
			func(
				_ *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				return starlark.MakeInt(7), nil
			},
		)
		env = starlark.StringDict{
			"plain": starlark.String("x"),
			"top":   inner,
			"mod": &starlarkstruct.Module{
				Name: "mod",
				Members: starlark.StringDict{
					"leaf": inner,
				},
			},
		}
		got = scheduler.Polled(env)
	)

	if len(got) != len(env) {
		t.Fatalf(
			"got %d names, want %d",
			len(got),
			len(env),
		)
	}

	if got["plain"] != env["plain"] {
		t.Fatalf(
			"plain value changed: %v",
			got["plain"],
		)
	}

	top, ok := got["top"].(*starlark.Builtin)
	if !ok || top.Name() != "leaf" {
		t.Fatalf(
			"top is %v, want a builtin named leaf",
			got["top"],
		)
	}

	module, ok := got["mod"].(*starlarkstruct.Module)
	if !ok || module.Name != "mod" {
		t.Fatalf(
			"mod is %v, want a module named mod",
			got["mod"],
		)
	}

	member, ok := module.Members["leaf"].(*starlark.Builtin)
	if !ok || member == inner {
		t.Fatalf(
			"member is %v, want a wrapped builtin",
			module.Members["leaf"],
		)
	}

	value, err := starlark.Call(
		&starlark.Thread{},
		member,
		nil,
		nil,
	)
	if err != nil || value.String() != "7" {
		t.Fatalf(
			"got %v, %v, want 7",
			value,
			err,
		)
	}

	if env["top"] != starlark.Value(inner) {
		t.Fatal("the original environment was changed")
	}
}
