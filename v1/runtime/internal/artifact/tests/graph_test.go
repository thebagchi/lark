package tests

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// CYCLE_A_FIXTURE is the entry of a load cycle: it loads cycle_b.star, which loads it
	// back.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/cycle_a.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	CYCLE_A_FIXTURE = `
load("cycle_b.star", "b")

def a():
    return b()

def main():
    return a()
`

	// CYCLE_B is the module that closes the cycle: it loads cycle_a.star.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/cycle_b.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	CYCLE_B = `
load("cycle_a.star", "a")

def b():
    return a()
`

	// BOTH is an entry that loads the side of left/ and the side of right/, two
	// modules that load one spelling of lib.star.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/both.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	BOTH = `
load("left/side.star", left = "side")
load("right/side.star", right = "side")

def main():
    return left() + right()
`

	// LEFT_LIB is left/lib.star, whose which returns "left".
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/left/lib.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LEFT_LIB = `
# Same spelling as right/lib.star, different file.
def which():
    return "left"
`

	// LEFT_SIDE is left/side.star, which loads lib.star beside it.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/left/side.star (C114,
	//     Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LEFT_SIDE = `
load("lib.star", "which")

def side():
    return which()
`

	// RIGHT_LIB is right/lib.star, whose which returns "right". It has the spelling of
	// LEFT_LIB and different text.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/right/lib.star (C114,
	//     Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	RIGHT_LIB = `
# Same spelling as left/lib.star, different file.
def which():
    return "right"
`

	// RIGHT_SIDE is right/side.star, which loads lib.star beside it.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/right/side.star (C114,
	//     Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	RIGHT_SIDE = `
load("lib.star", "which")

def side():
    return which()
`
)

const (
	EXPECTED_RING  = "cycle_a.star -> cycle_b.star -> cycle_a.star"
	EXPECTED_FETCH = 1
)

// _Files is a Loader that serves fixtures, counting what it was asked for so a
// test can tell one fetch from two.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Files struct {
	calls map[string]int
}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-09-19 21:44: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Files) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the fixture at name from FIXTURES, counting the read.
//
// Revisions:
//   - 2026-09-19 21:44: initial creation
//   - 2026-11-09 11:25: reads FIXTURES, not testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Files) Load(name string) ([]byte, error) {
	f.calls[name]++

	return FIXTURES.Load(name)
}

// _Loader returns a Loader over FIXTURES.
//
// Revisions:
//   - 2026-09-19 21:44: initial creation
//   - 2026-11-09 11:25: reads FIXTURES, not testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Loader() *_Files {
	return &_Files{
		calls: map[string]int{},
	}
}

// _Fixture returns the named script out of FIXTURES.
//
// Revisions:
//   - 2026-09-19 21:44: initial creation
//   - 2026-11-09 11:25: reads FIXTURES, not testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Fixture(
	t *testing.T,
	name string,
) []byte {
	t.Helper()

	src, ok := FIXTURES[name]
	if !ok {
		t.Fatalf(
			"no fixture %s",
			name,
		)
	}

	return []byte(src)
}

// TestCompile_AllowsAModuleWithoutAnEntryPoint proves a library compiles: what
// a run needs is not what a compile needs.
//
// Revisions:
//   - 2026-09-19 21:45: initial creation
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCompile_AllowsAModuleWithoutAnEntryPoint(t *testing.T) {
	_, err := artifact.Compile(&common.Source{
		Entry: "lib.star",
		Text: _Fixture(
			t,
			"lib.star",
		),
		Loader: _Loader(),
	})
	if err != nil {
		t.Fatalf(
			"compiling a library: %v",
			err,
		)
	}
}

// TestCompile_RefusesACycleWithoutRunningAnything proves a cycle is found from
// compiled code, before any top level executes, and costs no fetch of the
// module that closes it.
//
// Revisions:
//   - 2026-09-19 21:45: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCompile_RefusesACycleWithoutRunningAnything(t *testing.T) {
	var loader = _Loader()

	_, err := artifact.Compile(&common.Source{
		Entry: "cycle_a.star",
		Text: _Fixture(
			t,
			"cycle_a.star",
		),
		Loader: loader,
	})
	if !errors.Is(
		err,
		spelling.ErrCycle,
	) {
		t.Fatalf(
			"compiling a cycle gave %v, want ErrCycle",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		EXPECTED_RING,
	) {
		t.Fatalf(
			"refusal does not name the ring: %v",
			err,
		)
	}

	if loader.calls["cycle_a.star"] != 0 {
		t.Fatalf(
			"the module closing the cycle was fetched %d times",
			loader.calls["cycle_a.star"],
		)
	}

	t.Logf(
		"refused: %v",
		err,
	)
}

// TestCompile_FetchesEachModuleOnce proves two routes to one module produce one
// unit rather than two that can drift.
//
// Revisions:
//   - 2026-09-19 21:46: initial creation
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCompile_FetchesEachModuleOnce(t *testing.T) {
	var loader = _Loader()

	_, err := artifact.Compile(&common.Source{
		Entry: "app.star",
		Text: _Fixture(
			t,
			"app.star",
		),
		Loader: loader,
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	for _, name := range []string{"lib.star", "helper.star"} {
		if loader.calls[name] != EXPECTED_FETCH {
			t.Fatalf(
				"%s was fetched %d times, want %d",
				name,
				loader.calls[name],
				EXPECTED_FETCH,
			)
		}
	}
}

// TestCompile_ReadsModulesBesideTheScriptByDefault proves a source naming no
// Modules is compiled with every module it loads read as a file beside the one
// that loaded it.
//
// Revisions:
//   - 2026-09-19 21:46: initial creation, as TestCompile_RefusesALoadWithNoLoader
//   - 2026-10-03 00:14: a source with no Modules reads files beside the script,
//     there being no compile without a loader to refuse
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCompile_ReadsModulesBesideTheScriptByDefault(t *testing.T) {
	var dir = t.TempDir()

	for _, name := range []string{"app.star", "lib.star", "helper.star"} {
		err := os.WriteFile(
			filepath.Join(
				dir,
				name,
			),
			_Fixture(
				t,
				name,
			),
			0o600,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err := artifact.Compile(&common.Source{
		Entry: filepath.Join(
			dir,
			"app.star",
		),
		Text: _Fixture(
			t,
			"app.star",
		),
	})
	if err != nil {
		t.Fatalf(
			"compiling with modules beside the script: %v",
			err,
		)
	}
}

// TestCompile_MergesEveryModuleIntoOneUnit proves the artifact carries the
// whole graph in one unit and that the merge keeps each module. Row one: the
// script and every module it loads are merged, and the run sees them all. Row
// two: a module is identified by what the loader called it, not by how a script
// spelled it. left/side.star and right/side.star both load "lib.star" and must
// reach different files, so the merge keeps both and each side calls its own.
//
// Revisions:
//   - 2026-09-19 21:48: initial creation
//   - 2026-09-19 21:48: initial creation
//   - 2026-11-02 22:58: one unit, the entry, since a compile merges (C44a)
//   - 2026-11-02 22:58: reads the result of a run, since a compile merges
//     (C44a)
//   - 2026-11-08 22:47: moved to the external package, merged with
//     TestCompile_OneSpellingCanMeanTwoModules into one table; reads the units
//     through Save (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCompile_MergesEveryModuleIntoOneUnit(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		want    string
	}{
		{
			"every dependency is held",
			"app.star",
			"13",
		},
		{
			"one spelling can mean two modules",
			"both.star",
			"\"leftright\"",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				units := _Bundle(
					t,
					item.fixture,
				).GetUnits()

				if len(units) != 1 || units[0].GetName() != item.fixture {
					t.Fatalf(
						"artifact holds %d units, want the one unit %s",
						len(units),
						item.fixture,
					)
				}

				got, err := artifact.Run(
					t.Context(),
					_Built(
						t,
						item.fixture,
					),
				)
				if err != nil {
					t.Fatalf(
						"run: %v",
						err,
					)
				}

				if got.String() != item.want {
					t.Fatalf(
						"got %s, want %s: the merge lost a module",
						got,
						item.want,
					)
				}
			},
		)
	}
}
