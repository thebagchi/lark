package tests

import (
	"errors"
	"fmt"
	"path"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// _Map is a Loader backed by a map of path to source text.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Map struct {
	files map[string][]byte
}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m *_Map) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the text filed under name, or an error.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m *_Map) Load(name string) ([]byte, error) {
	src, ok := m.files[name]
	if !ok {
		return nil, fmt.Errorf(
			"no such module: %s",
			name,
		)
	}

	return src, nil
}

// TestRegression_Graph covers two findings in graph.Of that only the compile
// path tested before.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRegression_Graph(t *testing.T) {
	t.Run(
		"cycle",
		func(t *testing.T) {
			_Cycle(t)
		},
	)

	t.Run(
		"one spelling two modules",
		func(t *testing.T) {
			_TwoModules(t)
		},
	)
}

// _Cycle calls graph.Of on a script whose modules load each other. The walk
// must refuse the cycle with spelling.ErrCycle.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Cycle(t *testing.T) {
	t.Helper()

	var (
		entry = "cycle_a.star"
		text  = []byte(
			"load(\"cycle_b.star\", \"b\")\n\n" +
				"def b():\n    pass\n\n" +
				"def main():\n    b()\n",
		)
		loader = &_Map{
			files: map[string][]byte{
				"cycle_b.star": []byte(
					"load(\"cycle_a.star\", \"a\")\n\n" +
						"def a():\n    pass\n",
				),
			},
		}
		source = &common.Source{
			Entry:  entry,
			Text:   text,
			Loader: loader,
		}
	)

	_, err := graph.Of(source)
	if !errors.Is(
		err,
		spelling.ErrCycle,
	) {
		t.Fatalf(
			"got %v, want ErrCycle",
			err,
		)
	}

	t.Logf(
		"refused: %v",
		err,
	)
}

// _TwoModules calls graph.Of on a script where one spelling resolves to two
// modules. left/side.star and right/side.star both load "lib.star", and the
// resolver turns that into left/lib.star and right/lib.star. The flow must
// list the functions from both.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _TwoModules(t *testing.T) {
	t.Helper()

	var (
		entry = "entry.star"
		text  = []byte(
			"load(\"left/side.star\", \"go_left\")\n" +
				"load(\"right/side.star\", \"go_right\")\n\n" +
				"def main():\n    go_left()\n    go_right()\n",
		)
		loader = &_Map{
			files: map[string][]byte{
				"left/side.star": []byte(
					"load(\"lib.star\", \"help_left\")\n\n" +
						"def go_left():\n    help_left()\n",
				),
				"right/side.star": []byte(
					"load(\"lib.star\", \"help_right\")\n\n" +
						"def go_right():\n    help_right()\n",
				),
				"left/lib.star": []byte(
					"def help_left():\n    pass\n",
				),
				"right/lib.star": []byte(
					"def help_right():\n    pass\n",
				),
			},
		}
		source = &common.Source{
			Entry:  entry,
			Text:   text,
			Loader: loader,
		}
	)

	flow, err := graph.Of(source)
	if err != nil {
		t.Fatalf(
			"derive: %v",
			err,
		)
	}

	names := make(map[string]bool)
	for _, fn := range flow.GetFunctions() {
		names[fn.GetName()] = true
	}

	for _, want := range []string{
		"help_left",
		"help_right",
		"go_left",
		"go_right",
	} {
		if !names[want] {
			t.Fatalf(
				"flow does not list %s; functions: %v",
				want,
				names,
			)
		}
	}

	t.Logf(
		"one spelling, both modules: %v",
		names,
	)
}

// TestOf_RefusesLoadsWithTheMergedSentinels proves a load that renames matches
// ErrCarry, a name that two modules declare matches ErrDuplicate, and a
// top-level statement of a loaded module matches ErrCarry: the old
// the sentinels for an alias, a collision and a statement were merged into them.
//
// Revisions:
//   - 2026-11-08 13:25: initial creation (C98, Q250)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func TestOf_RefusesLoadsWithTheMergedSentinels(t *testing.T) {
	cases := []struct {
		name  string
		entry string
		files map[string]string
		want  error
	}{
		{
			name: "a load that renames",
			entry: "load(\"lib.star\", shout = \"yell\")\n\n" +
				"def main():\n    shout()\n",
			files: map[string]string{
				"lib.star": "def yell():\n    pass\n",
			},
			want: graph.ErrCarry,
		},
		{
			name: "two modules declare one name",
			entry: "load(\"a.star\", \"f\")\nload(\"b.star\", \"g\")\n\n" +
				"def main():\n    f()\n    g()\n",
			files: map[string]string{
				"a.star": "def f():\n    pass\n",
				"b.star": "def f():\n    pass\n\ndef g():\n    f()\n",
			},
			want: graph.ErrDuplicate,
		},
		{
			name:  "a statement in a loaded module",
			entry: "load(\"lib.star\", \"f\")\n\ndef main():\n    f()\n",
			files: map[string]string{
				"lib.star": "print(\"init\")\n\ndef f():\n    pass\n",
			},
			want: graph.ErrCarry,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					loader = &_Map{
						files: map[string][]byte{},
					}
					source = &common.Source{
						Entry:  "main.star",
						Text:   []byte(tc.entry),
						Loader: loader,
					}
				)

				for name, text := range tc.files {
					loader.files[name] = []byte(text)
				}

				_, err := graph.Of(source)
				if !errors.Is(
					err,
					tc.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tc.want,
					)
				}
			},
		)
	}
}
