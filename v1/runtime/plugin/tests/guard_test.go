package tests

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base32"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/hex"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/hash"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/json"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/math"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/path"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/random"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/regexp"
)

// WANTS_BYTES is the text integer.from_bytes gives for an argument that is not
// bytes.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	WANTS_BYTES = "wants bytes or a string"
)

// _FromEnvironment returns every builtin Environment gives for Registered, as a
// compile reads them, by dotted name: a top-level one by its name, a module
// member as module.member.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93), as _Builtins
//   - 2026-11-04 22:35: split from the Values reader (Q134)
//   - 2026-11-07 19:00: comment says Registered, not DEFAULT (C91w1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _FromEnvironment(t *testing.T) map[string]*starlark.Builtin {
	t.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	return _Members(env)
}

// _FromValues returns every builtin the Values of each plugin of Registered gives,
// as a Go caller reads them directly.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-07 19:00: comment says Registered, not DEFAULT (C91w1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _FromValues() map[string]*starlark.Builtin {
	names := starlark.StringDict{}

	for _, registered := range plugin.Registered() {
		maps.Copy(
			names,
			registered.Values(),
		)
	}

	return _Members(names)
}

// _Members is the builtins in names, by dotted name.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Members(names starlark.StringDict) map[string]*starlark.Builtin {
	found := map[string]*starlark.Builtin{}

	for name, value := range names {
		switch held := value.(type) {
		case *starlark.Builtin:
			found[name] = held

		case *starlarkstruct.Module:
			for member, inner := range held.Members {
				builtin, ok := inner.(*starlark.Builtin)
				if ok {
					found[name+"."+member] = builtin
				}
			}
		}
	}

	return found
}

// _Call calls fn from Go, as a host does. It returns the panic the call raised,
// as an error, or nil, and then the error the call returned.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Call(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (error, error) {
	var failure error

	raised := guard.Contained(func() {
		_, failure = starlark.Call(
			new(starlark.Thread),
			fn,
			args,
			kwargs,
		)
	})

	return raised, failure
}

// _Refuses fails t unless calling builtin with shape returns an error and
// raises no panic.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Refuses(
	t *testing.T,
	name string,
	builtin *starlark.Builtin,
	shape *_Shape,
) {
	t.Helper()

	raised, err := _Call(
		builtin,
		shape.args,
		shape.kwargs,
	)
	if raised != nil {
		t.Fatalf(
			"%s panicked: %v",
			name,
			raised,
		)
	}

	if err == nil {
		t.Fatalf(
			"%s returned no error",
			name,
		)
	}
}

// _Shape is one malformed call.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Shape struct {
	name   string
	args   starlark.Tuple
	kwargs []starlark.Tuple
}

// _Shapes are the calls that panic go.starlark.net's unpacking: a nil Value and
// a malformed keyword pair, in each of the ways one is made.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation, from the table of C93's first test
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Shapes() []_Shape {
	var (
		one  = starlark.MakeInt(1)
		name = starlark.String("n")
	)

	return []_Shape{
		{
			name: "nil argument alone",
			args: starlark.Tuple{nil},
		},
		{
			name: "nil argument after a good one",
			args: starlark.Tuple{one, nil},
		},
		{
			name:   "keyword pair of one element",
			kwargs: []starlark.Tuple{{name}},
		},
		{
			name:   "keyword pair of none",
			kwargs: []starlark.Tuple{{}},
		},
		{
			name:   "keyword nil pair",
			kwargs: []starlark.Tuple{nil},
		},
		{
			name:   "keyword name is not a string",
			kwargs: []starlark.Tuple{{one, one}},
		},
		{
			name:   "keyword name is nil",
			kwargs: []starlark.Tuple{{nil, one}},
		},
		{
			name:   "keyword value is nil",
			kwargs: []starlark.Tuple{{name, nil}},
		},
	}
}

// TestValues_BuiltinsRefuseWhatBreaksUnpack proves a builtin called from Go
// with a nil Value or a malformed keyword pair returns an error and does not
// panic, for every builtin of ten plugins, integer among them, whether a Go
// caller took it from a plugin's Values or from Environment.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93), as TestEnvironment_...
//   - 2026-11-04 22:35: also reads Values directly (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestValues_BuiltinsRefuseWhatBreaksUnpack(t *testing.T) {
	for _, source := range []struct {
		name  string
		fetch func(*testing.T) map[string]*starlark.Builtin
	}{
		{
			name:  "environment",
			fetch: _FromEnvironment,
		},
		{
			name: "values",
			fetch: func(*testing.T) map[string]*starlark.Builtin {
				return _FromValues()
			},
		},
	} {
		t.Run(
			source.name,
			func(t *testing.T) {
				builtins := source.fetch(t)

				for _, required := range []string{
					"integer.from_bytes",
					"integer.to_bytes",
				} {
					if builtins[required] == nil {
						t.Fatalf(
							"no %s among %d builtins",
							required,
							len(builtins),
						)
					}
				}

				for _, name := range slices.Sorted(maps.Keys(builtins)) {
					for _, shape := range _Shapes() {
						t.Run(
							fmt.Sprintf(
								"%s/%s",
								name,
								shape.name,
							),
							func(t *testing.T) {
								_Refuses(
									t,
									name,
									builtins[name],
									&shape,
								)
							},
						)
					}
				}
			},
		)
	}
}

// _Host is a plugin of one builtin that unpacks its argument as a host's may.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Host() *_Fixed {
	host := &_Fixed{
		name: "host",
		values: starlark.StringDict{
			"fragile": starlark.NewBuiltin(
				"fragile",
				func(
					_ *starlark.Thread,
					fn *starlark.Builtin,
					args starlark.Tuple,
					kwargs []starlark.Tuple,
				) (starlark.Value, error) {
					var text string

					err := starlark.UnpackPositionalArgs(
						fn.Name(),
						args,
						kwargs,
						1,
						&text,
					)

					return starlark.None, err
				},
			),
		},
	}

	return host
}

// TestEnvironment_AHostPluginIsGuarded proves a plugin that never went through
// Register, as a host's does not, has its builtins checked too.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnvironment_AHostPluginIsGuarded(t *testing.T) {
	env, err := plugin.Environment([]common.Plugin{_Host()})
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	fragile, ok := env["fragile"].(*starlark.Builtin)
	if !ok {
		t.Fatal("fragile is not a builtin")
	}

	raised, failure := _Call(
		fragile,
		starlark.Tuple{nil},
		nil,
	)
	if raised != nil || failure == nil {
		t.Fatalf(
			"raised = %v, failure = %v",
			raised,
			failure,
		)
	}
}

// TestEnvironment_GuardKeepsTheCallAsItWas proves the seam changes nothing for a
// call that is well formed: the result, the error text of a bad call and the
// name are those of the builtin it wraps.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnvironment_GuardKeepsTheCallAsItWas(t *testing.T) {
	var (
		builtins = _FromEnvironment(t)
		from     = builtins["integer.from_bytes"]
	)

	if from.Name() != "integer.from_bytes" {
		t.Fatalf(
			"name = %q",
			from.Name(),
		)
	}

	got, err := starlark.Call(
		new(starlark.Thread),
		from,
		starlark.Tuple{starlark.Bytes("\x01\x02")},
		nil,
	)
	if err != nil {
		t.Fatalf(
			"from_bytes: %v",
			err,
		)
	}

	if got.String() != "258" {
		t.Fatalf(
			"from_bytes = %s, want 258",
			got,
		)
	}

	_, err = starlark.Call(
		new(starlark.Thread),
		from,
		starlark.Tuple{starlark.MakeInt(1)},
		nil,
	)
	if err == nil {
		t.Fatal("from_bytes of an int returned no error")
	}

	if !strings.Contains(
		err.Error(),
		WANTS_BYTES,
	) {
		t.Fatalf(
			"error = %v, want it to hold %q",
			err,
			WANTS_BYTES,
		)
	}
}
