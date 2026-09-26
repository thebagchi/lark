package artifact_test

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
)

const (
	SAVE_FIXTURE      = "app.star"
	UNCARRIED_FIXTURE = "uncarried.star"
	SAVED_UNITS       = 3
	FIRST_SAVED       = "lib.star"
	LAST_SAVED        = "app.star"
	EXPECTED_EDGE     = "lib.star"

	// GRAPHED is a fixture whose graph derives, and the function it defines
	// that a bundle should name without carrying its text.
	GRAPHED      = "concurrent.star"
	GRAPHED_FUNC = "left"
)

// _Program decodes compiled bytes back through the interpreter's own reader.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
func _Program(code []byte) (*starlark.Program, error) {
	return starlark.CompiledProgram(strings.NewReader(string(code)))
}

// _Bundle saves the named fixture and decodes the bundle it produced.
//
// Revisions:
//   - 2026-09-19 23:26: initial creation, as _Decoded over one message per unit
//   - 2026-09-21 17:19: one bundle, which is what Save now returns
func _Bundle(t *testing.T, name string) *artifactpb.Artifact {
	t.Helper()

	saved, err := _Built(t, name).Save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	bundle := new(artifactpb.Artifact)

	err = proto.Unmarshal(saved, bundle)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	return bundle
}

// TestSave_CarriesWhatAContainerNeeds proves a bundle carries the four things
// nothing downstream can recompute: which unit is the entry, the name each is
// filed under, its compiled code, and what each load spelling resolved to.
//
// Resolving a spelling is a loader's job, and whatever reads these bytes has
// none - which is why the resolutions are carried rather than derived.
//
// Revisions:
//   - 2026-09-19 23:27: initial creation
//   - 2026-09-21 17:19: reads one bundle
func TestSave_CarriesWhatAContainerNeeds(t *testing.T) {
	bundle := _Bundle(t, SAVE_FIXTURE)

	if bundle.GetEntry() != LAST_SAVED {
		t.Fatalf("entry is %q, want %q", bundle.GetEntry(), LAST_SAVED)
	}

	units := bundle.GetUnits()

	if len(units) != SAVED_UNITS {
		t.Fatalf("saved %d units, want %d", len(units), SAVED_UNITS)
	}

	var entry *artifactpb.Unit

	for _, unit := range units {
		if unit.GetName() == "" {
			t.Fatal("a saved unit has no name")
		}

		if len(unit.GetCode()) == 0 {
			t.Fatalf("unit %s carries no code", unit.GetName())
		}

		if unit.GetName() == LAST_SAVED {
			entry = unit
		}
	}

	if entry == nil {
		t.Fatalf("saved units do not include %s", LAST_SAVED)
	}

	if entry.GetLoads()[EXPECTED_EDGE] != EXPECTED_EDGE {
		t.Fatalf(
			"%s resolved to %q, want %q",
			EXPECTED_EDGE,
			entry.GetLoads()[EXPECTED_EDGE],
			EXPECTED_EDGE,
		)
	}
}

// TestSave_IsInInitialisationOrder proves a reader can replay the units rather
// than sort them: a dependency is always saved before whatever loads it.
//
// Revisions:
//   - 2026-09-19 23:28: initial creation
func TestSave_IsInInitialisationOrder(t *testing.T) {
	units := _Bundle(t, SAVE_FIXTURE).GetUnits()

	seen := map[string]bool{}

	for _, unit := range units {
		for spelling, name := range unit.GetLoads() {
			if !seen[name] {
				t.Fatalf(
					"%s loads %q, resolved to %s, "+
						"which is not saved before it",
					unit.GetName(),
					spelling,
					name,
				)
			}
		}

		seen[unit.GetName()] = true
	}

	if units[0].GetName() != FIRST_SAVED || units[len(units)-1].GetName() != LAST_SAVED {
		t.Fatalf("saved order is not dependencies first: %v", _Names(units))
	}
}

// _Names returns the names of units, for a failure message.
//
// Revisions:
//   - 2026-09-19 23:28: initial creation
func _Names(units []*artifactpb.Unit) []string {
	names := make([]string, 0, len(units))

	for _, unit := range units {
		names = append(names, unit.GetName())
	}

	return names
}

// TestSave_EveryUnitRoundTripsAsAProgram proves the bytes are a compiled
// program and not something that merely unmarshals: each one decodes back
// through the interpreter's own reader.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
func TestSave_EveryUnitRoundTripsAsAProgram(t *testing.T) {
	for _, unit := range _Bundle(t, SAVE_FIXTURE).GetUnits() {
		program, err := _Program(unit.GetCode())
		if err != nil {
			t.Fatalf("decode %s: %v", unit.GetName(), err)
		}

		if program.NumLoads() != len(unit.GetLoads()) {
			t.Fatalf(
				"%s: the program lists %d loads, the unit carries %d",
				unit.GetName(),
				program.NumLoads(),
				len(unit.GetLoads()),
			)
		}
	}
}

// TestSave_CarriesTheGraphWithoutBodies is what a bundle gained: the shape a
// user interface draws, and not a second copy of the program.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func TestSave_CarriesTheGraphWithoutBodies(t *testing.T) {
	described := _Bundle(t, GRAPHED).GetGraph()

	if described == nil {
		t.Fatal("want the graph in the bundle")
	}

	if len(described.GetThreads()) == 0 {
		t.Fatal("want the threads a user interface draws")
	}

	named := false

	for _, fn := range described.GetFunctions() {
		if fn.GetBody() != "" {
			t.Fatalf("%s carries its body, which the compiled code already is",
				fn.GetName())
		}

		if fn.GetName() == GRAPHED_FUNC {
			named = true
		}
	}

	if !named {
		t.Fatalf("want %s among the functions the graph names", GRAPHED_FUNC)
	}
}

// TestCompile_AScriptNoGraphDescribesStillRuns is why deriving never fails a
// compile: a script that runs may still be one no graph can describe, and
// refusing to compile it would let a display concern decide whether a program
// may run.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation, as TestCompile_RecordsAGraphItCouldNotCarry
//   - 2026-09-21 23:47: a bundle either has a graph or has none, so there is
//     no record to read
func TestCompile_AScriptNoGraphDescribesStillRuns(t *testing.T) {
	built := _Built(t, UNCARRIED_FIXTURE)

	if built.Graph() != nil {
		t.Fatal("want no graph for a script no graph describes")
	}

	if _, err := built.Run(t.Context()); err != nil {
		t.Fatalf("want a script with no graph to run: %v", err)
	}

	// And the bundle is still a bundle: the program is what it carries.
	if len(_Bundle(t, UNCARRIED_FIXTURE).GetUnits()) == 0 {
		t.Fatal("want the compiled units")
	}
}
