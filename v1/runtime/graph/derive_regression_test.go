package graph_test

import (
	"errors"
	"fmt"
	"path"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/graph"
	"github.com/thebagchi/lark/v1/runtime/script"
)

// _Map is a Loader backed by a map of path to source text.
type _Map struct {
	files map[string][]byte
}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
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
// must refuse the cycle with ErrCycle.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
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
		source = &script.Source{
			Entry:  entry,
			Text:   text,
			Loader: loader,
		}
	)

	_, err := graph.Of(source)
	if !errors.Is(
		err,
		graph.ErrCycle,
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
		source = &script.Source{
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
