package tests

import (
	"bytes"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

const (
	SAVED_UNITS = 1
	FIRST_SAVED = "app.star"
	LAST_SAVED  = "app.star"
	SAVE_BUILDS = 20
)

// _Program decodes compiled bytes back through the interpreter's own reader.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Program(code []byte) (*starlark.Program, error) {
	return starlark.CompiledProgram(strings.NewReader(string(code)))
}

// _Bundle saves the named fixture and decodes the bundle it produced.
//
// Revisions:
//   - 2026-09-19 23:26: initial creation, as _Decoded over one message per unit
//   - 2026-09-21 17:19: one bundle, which is what Save now returns
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Bundle(
	t *testing.T,
	name string,
) *artifactpb.Artifact {
	t.Helper()

	saved, err := _Built(
		t,
		name,
	).Save()
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	bundle := new(artifactpb.Artifact)

	err = proto.Unmarshal(
		saved,
		bundle,
	)
	if err != nil {
		t.Fatalf(
			"unmarshal: %v",
			err,
		)
	}

	return bundle
}

// TestSave_CarriesWhatAContainerNeeds proves a bundle carries what nothing
// downstream can recompute: which unit is the entry, the name it is filed
// under and its compiled code. A compile merges every load into the entry, so
// the unit carries no load. The format still holds a map of resolutions for a
// bundle of many units, which TestLoad_* build by hand.
//
// Revisions:
//   - 2026-09-19 23:27: initial creation
//   - 2026-09-21 17:19: reads one bundle
//   - 2026-11-02 22:58: one unit, with no load (C44a)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSave_CarriesWhatAContainerNeeds(t *testing.T) {
	bundle := _Bundle(
		t,
		"app.star",
	)

	if bundle.GetEntry() != LAST_SAVED {
		t.Fatalf(
			"entry is %q, want %q",
			bundle.GetEntry(),
			LAST_SAVED,
		)
	}

	units := bundle.GetUnits()

	if len(units) != SAVED_UNITS {
		t.Fatalf(
			"saved %d units, want %d",
			len(units),
			SAVED_UNITS,
		)
	}

	var entry *artifactpb.Unit

	for _, unit := range units {
		if unit.GetName() == "" {
			t.Fatal("a saved unit has no name")
		}

		if len(unit.GetCode()) == 0 {
			t.Fatalf(
				"unit %s carries no code",
				unit.GetName(),
			)
		}

		if unit.GetName() == LAST_SAVED {
			entry = unit
		}
	}

	if entry == nil {
		t.Fatalf(
			"saved units do not include %s",
			LAST_SAVED,
		)
	}

	if len(entry.GetLoads()) != 0 {
		t.Fatalf(
			"%s carries loads %v, want none: a compile merges them",
			LAST_SAVED,
			entry.GetLoads(),
		)
	}
}

// TestSave_IsInInitialisationOrder proves a reader can replay the units rather
// than sort them: a dependency is always saved before whatever loads it.
//
// Revisions:
//   - 2026-09-19 23:28: initial creation
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSave_IsInInitialisationOrder(t *testing.T) {
	var (
		units = _Bundle(
			t,
			"app.star",
		).GetUnits()
		seen = map[string]bool{}
	)

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
		t.Fatalf(
			"saved order is not dependencies first: %v",
			_Names(units),
		)
	}
}

// _Names returns the names of units, for a failure message.
//
// Revisions:
//   - 2026-09-19 23:28: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Names(units []*artifactpb.Unit) []string {
	names := make(
		[]string,
		0,
		len(units),
	)

	for _, unit := range units {
		names = append(
			names,
			unit.GetName(),
		)
	}

	return names
}

// TestSave_EveryUnitRoundTripsAsAProgram proves the bytes are a compiled
// program and not something that merely unmarshals: each one decodes back
// through the interpreter's own reader.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSave_EveryUnitRoundTripsAsAProgram(t *testing.T) {
	for _, unit := range _Bundle(
		t,
		"app.star",
	).GetUnits() {
		program, err := _Program(unit.GetCode())
		if err != nil {
			t.Fatalf(
				"decode %s: %v",
				unit.GetName(),
				err,
			)
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

// TestSave_BundleBytesAreStable proves that building the same six-module
// artifact many times gives the same bundle bytes. A unit that loads two
// modules carries a map of load spellings, and protobuf orders map entries at
// random unless the marshal is deterministic.
//
// The guarantee holds for one binary. It is not a canonical form across
// languages or library versions (protobuf v1.36.12 proto/encode.go:36-55).
//
// Revisions:
//   - 2026-11-02 08:32: initial creation, for plan question Q6
//   - 2026-11-02 10:25: six loads, so a chance pass is below 1 in 10,000
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSave_BundleBytesAreStable(t *testing.T) {
	var first []byte

	for build := 0; build < SAVE_BUILDS; build++ {
		saved, err := _Built(
			t,
			"spread.star",
		).Save()
		if err != nil {
			t.Fatalf(
				"save %d: %v",
				build,
				err,
			)
		}

		if first == nil {
			first = saved

			continue
		}

		if !bytes.Equal(
			first,
			saved,
		) {
			t.Fatalf(
				"build %d gave different bundle bytes",
				build,
			)
		}
	}
}

// TestSave_CarriesTheMergedSource proves a bundle carries the text of the
// merge beside the code: the script and the modules it loads, with no load
// left, and the same bytes on every save.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a, plan Q38)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSave_CarriesTheMergedSource(t *testing.T) {
	var (
		built = _Built(
			t,
			"app.star",
		)
		first, err = built.Save()
	)
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	again, err := built.Save()
	if err != nil {
		t.Fatalf(
			"save again: %v",
			err,
		)
	}

	if !bytes.Equal(
		first,
		again,
	) {
		t.Fatal("two saves of one artifact differ")
	}

	bundle := new(artifactpb.Artifact)

	err = proto.Unmarshal(
		first,
		bundle,
	)
	if err != nil {
		t.Fatalf(
			"unmarshal: %v",
			err,
		)
	}

	source := bundle.GetUnits()[0].GetSource()

	for _, want := range []string{"def double", "def triple", "def main"} {
		if !strings.Contains(
			source,
			want,
		) {
			t.Errorf(
				"the carried source lacks %q:\n%s",
				want,
				source,
			)
		}
	}

	if strings.Contains(
		source,
		"load(",
	) {
		t.Errorf(
			"a load is left in the carried source:\n%s",
			source,
		)
	}
}

// TestLoad_SourceRoundTrips proves a bundle with source loads and keeps the
// source through the wire, and a bundle written before the field existed,
// with no source, still loads and runs with an empty source.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a, plan Q47)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_SourceRoundTrips(t *testing.T) {
	var (
		saved, err = _Built(
			t,
			"app.star",
		).Save()
		bundle = new(artifactpb.Artifact)
	)
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	err = proto.Unmarshal(
		saved,
		bundle,
	)
	if err != nil {
		t.Fatalf(
			"unmarshal: %v",
			err,
		)
	}

	t.Run(
		"with source",
		func(t *testing.T) {
			if bundle.GetUnits()[0].GetSource() == "" {
				t.Fatal("the saved unit carries no source")
			}

			again, err := proto.Marshal(bundle)
			if err != nil {
				t.Fatalf(
					"marshal: %v",
					err,
				)
			}

			back := new(artifactpb.Artifact)

			err = proto.Unmarshal(
				again,
				back,
			)
			if err != nil {
				t.Fatalf(
					"unmarshal: %v",
					err,
				)
			}

			if back.GetUnits()[0].GetSource() != bundle.GetUnits()[0].GetSource() {
				t.Fatal("the source changed on the wire")
			}

			_, err = artifact.Load(again)
			if err != nil {
				t.Fatalf(
					"load: %v",
					err,
				)
			}
		},
	)

	t.Run(
		"without source",
		func(t *testing.T) {
			old := proto.Clone(bundle).(*artifactpb.Artifact)

			for _, unit := range old.GetUnits() {
				unit.Source = ""
			}

			raw, err := proto.Marshal(old)
			if err != nil {
				t.Fatalf(
					"marshal: %v",
					err,
				)
			}

			read := new(artifactpb.Artifact)

			err = proto.Unmarshal(
				raw,
				read,
			)
			if err != nil {
				t.Fatalf(
					"unmarshal: %v",
					err,
				)
			}

			if read.GetUnits()[0].GetSource() != "" {
				t.Fatal("an old bundle should have an empty source")
			}

			_, err = artifact.Load(raw)
			if err != nil {
				t.Fatalf(
					"an old bundle should load: %v",
					err,
				)
			}
		},
	)
}
