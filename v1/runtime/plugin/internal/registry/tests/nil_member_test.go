package tests

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/registry"
)

// _Holding is a plugin that supplies the values it was given.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B248, B249, B250)
type _Holding struct {
	name  string
	given starlark.StringDict
}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B248, B249, B250)
func (h *_Holding) Name() string {
	if h.name != "" {
		return h.name
	}

	return "holding"
}

// Values returns the values it was given.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B248, B249, B250)
func (h *_Holding) Values() starlark.StringDict {
	return h.given
}

// _Breaking is a value whose Freeze panics.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B249)
type _Breaking struct {
	starlark.NoneType
}

// Freeze panics.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B249)
func (b *_Breaking) Freeze() {
	panic("freeze broke")
}

// _Nils is the values a plugin may not supply: a nil, and a nil pointer of
// each kind that Freeze, the wrapper or a call dereferences.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B248, B249, B250)
func _Nils() map[string]starlark.StringDict {
	var (
		module  *starlarkstruct.Module
		builtin *starlark.Builtin
		list    *starlark.List
	)

	return map[string]starlark.StringDict{
		"a nil value":         {"x": nil},
		"a typed-nil module":  {"m": module},
		"a typed-nil builtin": {"b": builtin},
		"a typed-nil list":    {"l": list},
		"a nil module member": {"m": &starlarkstruct.Module{
			Name:    "m",
			Members: starlark.StringDict{"x": nil},
		}},
		"a typed-nil member": {"m": &starlarkstruct.Module{
			Name:    "m",
			Members: starlark.StringDict{"b": builtin},
		}},
		"a nil member nested": {"m": &starlarkstruct.Module{
			Name: "m",
			Members: starlark.StringDict{"n": &starlarkstruct.Module{
				Name:    "n",
				Members: starlark.StringDict{"x": nil},
			}},
		}},
		"a nil beside a valid": {"ok": starlark.MakeInt(1), "x": nil},
	}
}

// TestEnvironment_RefusesANilValue proves a plugin that supplies a nil value, a
// typed-nil module, builtin or list, or a module with a nil member, gives an
// error that names the plugin and the name from Environment, and no panic. A
// typed-nil builtin no longer reaches a script, where the call panicked
// (B248, B250, Q65, Q206).
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B248, B250)
func TestEnvironment_RefusesANilValue(t *testing.T) {
	for name, values := range _Nils() {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := registry.Environment(
					[]common.Plugin{&_Holding{
						given: values,
					}},
				)
				if err == nil {
					t.Fatal("Environment accepted a nil value")
				}

				if !strings.HasPrefix(
					err.Error(),
					"holding: plugin name ",
				) {
					t.Fatalf(
						"error %q does not name the plugin and the name",
						err,
					)
				}
			},
		)
	}
}

// TestEnvironment_ARegisteredPluginFailsEveryTime proves a registered plugin
// whose first build fails fails on every call. A nil member used to panic in
// Freeze inside sync.Once.Do, and the next call found the Once spent and
// returned the plugin as an empty success. A member whose Freeze panics is the
// same hole and is contained (B249, Q2, Q65).
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B249)
func TestEnvironment_ARegisteredPluginFailsEveryTime(t *testing.T) {
	cases := map[string]starlark.StringDict{
		"a nil member":                 {"x": nil},
		"a member whose Freeze panics": {"x": new(_Breaking)},
	}

	for name, values := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				before := len(registry.DEFAULT)

				registry.Register(&_Holding{
					given: values,
				})

				defer func() {
					registry.DEFAULT = registry.DEFAULT[:before]
				}()

				for call := range 3 {
					_, err := registry.Environment(registry.Registered())
					if err == nil {
						t.Fatalf(
							"call %d: failed plugin gave no error",
							call+1,
						)
					}
				}
			},
		)
	}
}

// TestEnvironment_BoundsNames proves a plugin name and a clashing name of
// 100,000 bytes are cut in the errors of Environment (Q4, B239).
//
// Revisions:
//   - 2026-11-10 16:06: initial creation (C117, B239)
func TestEnvironment_BoundsNames(t *testing.T) {
	var (
		long = strings.Repeat(
			"p",
			100000,
		)
		one = &_Holding{
			name:  long,
			given: starlark.StringDict{long: starlark.None},
		}
		two = &_Holding{
			name:  long + "2",
			given: starlark.StringDict{long: starlark.None},
		}
		nilled = &_Holding{
			name:  long,
			given: starlark.StringDict{"x": nil},
		}
	)

	cases := map[string][]common.Plugin{
		"a clash":      {one, two},
		"a nil member": {nilled},
	}

	for name, plugins := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := registry.Environment(plugins)
				if err == nil {
					t.Fatal("no error")
				}

				if len(err.Error()) > 1000 ||
					!strings.Contains(
						err.Error(),
						"... (100000 bytes)",
					) {
					t.Fatalf(
						"error is %d bytes, want a cut name: %.200q",
						len(err.Error()),
						err,
					)
				}
			},
		)
	}
}
