package artifact_test

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
)

// _Program decodes compiled bytes back through the interpreter's own reader.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
func _Program(code []byte) (*starlark.Program, error) {
	return starlark.CompiledProgram(strings.NewReader(string(code)))
}

const (
	SAVE_FIXTURE  = "app.star"
	SAVED_UNITS   = 3
	FIRST_SAVED   = "lib.star"
	LAST_SAVED    = "app.star"
	HELPER_SAVED  = "helper.star"
	EXPECTED_EDGE = "lib.star"
)

// _Decoded unmarshals every saved unit, in the order Save produced them.
//
// Revisions:
//   - 2026-09-19 23:26: initial creation
func _Decoded(t *testing.T, saved [][]byte) []*artifactpb.Unit {
	t.Helper()

	units := make([]*artifactpb.Unit, 0, len(saved))

	for index, encoded := range saved {
		message := &artifactpb.Unit{}

		err := proto.Unmarshal(encoded, message)
		if err != nil {
			t.Fatalf("unmarshal unit %d: %v", index, err)
		}

		units = append(units, message)
	}

	return units
}

// TestSave_CarriesWhatAContainerNeeds proves a saved unit carries the three
// things nothing downstream can recompute: the name the artifact filed it
// under, its compiled code, and what each load spelling in it resolved to.
//
// Resolving a spelling is a loader's job, and whatever reads these bytes has
// none - which is why the resolutions are carried rather than derived.
//
// Revisions:
//   - 2026-09-19 23:27: initial creation
func TestSave_CarriesWhatAContainerNeeds(t *testing.T) {
	saved, err := _Built(t, SAVE_FIXTURE).Save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	units := _Decoded(t, saved)

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

	t.Logf("%s carries loads %v", LAST_SAVED, entry.GetLoads())
}

// TestSave_IsInInitialisationOrder proves a reader can replay the slice rather
// than sort it: a dependency is always saved before whatever loads it.
//
// Revisions:
//   - 2026-09-19 23:28: initial creation
func TestSave_IsInInitialisationOrder(t *testing.T) {
	saved, err := _Built(t, SAVE_FIXTURE).Save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	units := _Decoded(t, saved)

	seen := map[string]bool{}

	for _, unit := range units {
		for spelling, name := range unit.GetLoads() {
			if !seen[name] {
				t.Fatalf(
					"%s loads %q, resolved to %s, which is not saved before it",
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
	saved, err := _Built(t, SAVE_FIXTURE).Save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	for _, unit := range _Decoded(t, saved) {
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
