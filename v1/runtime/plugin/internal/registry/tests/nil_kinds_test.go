package tests

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/registry"
)

// _FuncValue is a Starlark value that is a function, which may be nil.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
type _FuncValue func()

// String is the text of the value.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _FuncValue) String() string {
	return "func"
}

// Type is the name of the type.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _FuncValue) Type() string {
	return "func"
}

// Freeze does nothing.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _FuncValue) Freeze() {
	// Nothing to freeze.
}

// Truth is true.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _FuncValue) Truth() starlark.Bool {
	return true
}

// Hash is refused, as for a function.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _FuncValue) Hash() (uint32, error) {
	return 0, nil
}

// _MapValue is a Starlark value that is a map, which may be nil.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
type _MapValue map[string]int

// String is the text of the value.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _MapValue) String() string {
	return "map"
}

// Type is the name of the type.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _MapValue) Type() string {
	return "map"
}

// Freeze does nothing.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _MapValue) Freeze() {
	// Nothing to freeze.
}

// Truth is true.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _MapValue) Truth() starlark.Bool {
	return true
}

// Hash is refused, as for a map.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _MapValue) Hash() (uint32, error) {
	return 0, nil
}

// _SliceValue is a Starlark value that is a slice, which may be nil.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
type _SliceValue []int

// String is the text of the value.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _SliceValue) String() string {
	return "slice"
}

// Type is the name of the type.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _SliceValue) Type() string {
	return "slice"
}

// Freeze does nothing.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _SliceValue) Freeze() {
	// Nothing to freeze.
}

// Truth is true.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _SliceValue) Truth() starlark.Bool {
	return true
}

// Hash is refused, as for a slice.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func (v _SliceValue) Hash() (uint32, error) {
	return 0, nil
}

// _Quiet is a function that is there and does nothing.
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func _Quiet() {
	// Nothing to do.
}

// TestEnvironment_RefusesANilOfAnyKind proves a plugin that supplies a nil
// function, map or slice value, directly or as a module member, gets the
// refusal a nil pointer gets: an error that names the plugin and the name, and
// no value in the environment (B267, Q65, Q206).
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func TestEnvironment_RefusesANilOfAnyKind(t *testing.T) {
	var (
		fn    _FuncValue
		table _MapValue
		slice _SliceValue
	)

	cases := map[string]starlark.StringDict{
		"a nil function": {"x": fn},
		"a nil map":      {"x": table},
		"a nil slice":    {"x": slice},
		"a nil function member": {"m": &starlarkstruct.Module{
			Name:    "m",
			Members: starlark.StringDict{"x": fn},
		}},
		"a nil map member": {"m": &starlarkstruct.Module{
			Name:    "m",
			Members: starlark.StringDict{"x": table},
		}},
	}

	for name, values := range cases {
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

// TestEnvironment_AcceptsWhatIsThere proves a function, a map and a slice that
// are made, and a nil tuple, which is the empty tuple, are values a plugin may
// supply: only a nil that would be dereferenced is refused (B267).
//
// Revisions:
//   - 2026-11-11 00:38: initial creation (C122, B267)
func TestEnvironment_AcceptsWhatIsThere(t *testing.T) {
	var (
		values = starlark.StringDict{
			"fn":    _FuncValue(_Quiet),
			"table": _MapValue{},
			"slice": _SliceValue{},
			"tuple": starlark.Tuple(nil),
		}
		env, err = registry.Environment(
			[]common.Plugin{&_Holding{
				given: values,
			}},
		)
	)
	if err != nil {
		t.Fatalf(
			"got %v, want the values",
			err,
		)
	}

	for name := range values {
		_, found := env[name]
		if !found {
			t.Fatalf(
				"%s is not in the environment",
				name,
			)
		}
	}
}
