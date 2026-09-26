// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: the order a graph initialises in, and which units it holds, are internal
// state until Save exposes them - and Save is a later phase. Asserting the
// order from outside would mean shipping an accessor this phase has no caller
// for, which CLAUDE.md's dead-code rule forbids more strongly than go.md
// prefers the external test package.
package artifact

import (
	"os"
	"path"
	"path/filepath"
	"testing"
)

const (
	INTERNAL_DIR   = "testdata"
	ENTRY_FIXTURE  = "app.star"
	BOTH_FIXTURE   = "both.star"
	FIRST_UNIT     = "lib.star"
	LAST_UNIT      = "app.star"
	EXPECTED_UNITS = 3
	LEFT_LIB       = "left/lib.star"
	RIGHT_LIB      = "right/lib.star"
)

// _Disk is a Loader over testdata.
type _Disk struct{}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-09-19 21:47: initial creation
func (d *_Disk) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load reads the fixture at name.
//
// Revisions:
//   - 2026-09-19 21:47: initial creation
func (d *_Disk) Load(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(INTERNAL_DIR, name))
}

// _Names returns the unit names of built, in initialisation order.
//
// Revisions:
//   - 2026-09-19 21:47: initial creation
func _Names(built *Artifact) []string {
	names := make([]string, 0, len(built.saved.GetUnits()))

	for _, unit := range built.saved.GetUnits() {
		names = append(names, unit.GetName())
	}

	return names
}

// _Build compiles the named fixture or ends the test.
//
// Revisions:
//   - 2026-09-19 21:47: initial creation
func _Build(t *testing.T, name string) *Artifact {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(INTERNAL_DIR, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	built, err := NewCompiler(WithLoader(&_Disk{})).Compile(name, src)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}

	return built
}

// TestCompile_HoldsEveryDependency proves the artifact carries the whole graph,
// dependencies first, not just the script it was asked for.
//
// Revisions:
//   - 2026-09-19 21:48: initial creation
func TestCompile_HoldsEveryDependency(t *testing.T) {
	names := _Names(_Build(t, ENTRY_FIXTURE))

	if len(names) != EXPECTED_UNITS {
		t.Fatalf("artifact holds %v, want %d units", names, EXPECTED_UNITS)
	}

	if names[0] != FIRST_UNIT {
		t.Fatalf("units are %v, want a dependency first", names)
	}

	if names[len(names)-1] != LAST_UNIT {
		t.Fatalf("units are %v, want the entry last", names)
	}

	t.Logf("compiled together, dependencies first: %v", names)
}

// TestCompile_OneSpellingCanMeanTwoModules proves a module is identified by
// what the loader called it, not by how a script spelled it: left/side.star and
// right/side.star both load "lib.star" and must reach different files.
//
// Revisions:
//   - 2026-09-19 21:48: initial creation
func TestCompile_OneSpellingCanMeanTwoModules(t *testing.T) {
	names := _Names(_Build(t, BOTH_FIXTURE))

	for _, want := range []string{LEFT_LIB, RIGHT_LIB} {
		found := false

		for _, name := range names {
			if name == want {
				found = true
			}
		}

		if !found {
			t.Fatalf("units %v do not include %s", names, want)
		}
	}

	t.Logf("one spelling, two units: %v", names)
}
