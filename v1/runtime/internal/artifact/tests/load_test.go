package tests

import (
	"errors"
	"regexp"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// LOADED_SUM is what app.star returns: double(2) from lib.star and triple(3)
// from helper.star.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	LOADED_SUM = 13
)

// TestLoad_RunsWhatSaveWrote proves a bundle is a program: saved, read back
// with nothing but the bytes, and run to the answer the compiled artifact gives,
// the modules it loads included.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_RunsWhatSaveWrote(t *testing.T) {
	saved, err := _Built(
		t,
		"app.star",
	).Save()
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	loaded, err := artifact.Load(saved)
	if err != nil {
		t.Fatalf(
			"load: %v",
			err,
		)
	}

	value, err := artifact.Run(
		t.Context(),
		loaded,
	)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if got := _Number(
		t,
		value,
	); got != LOADED_SUM {
		t.Fatalf(
			"got %d, want %d",
			got,
			LOADED_SUM,
		)
	}
}

// TestLoad_RefusesABundleMissingAUnit proves a bundle that does not hold a
// module it loads is refused when it is read, rather than partway through a
// run.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func TestLoad_RefusesABundleMissingAUnit(t *testing.T) {
	bundle := _Handmade(
		t,
		"app.star",
	)
	bundle.Units = bundle.GetUnits()[1:]

	short, err := proto.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	_, err = artifact.Load(short)
	if !errors.Is(
		err,
		artifact.ErrUnit,
	) {
		t.Fatalf(
			"got %v, want ErrUnit",
			err,
		)
	}
}

// TestLoad_RefusesWhatIsNotABundle proves bytes that are not a bundle are
// refused, and so is an empty one, which names no entry.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func TestLoad_RefusesWhatIsNotABundle(t *testing.T) {
	_, err := artifact.Load([]byte("not a bundle"))
	if err == nil {
		t.Fatal("want bytes that are not a bundle refused")
	}

	_, err = artifact.Load(nil)
	if !errors.Is(
		err,
		artifact.ErrUnit,
	) {
		t.Fatalf(
			"an empty bundle gave %v, want ErrUnit",
			err,
		)
	}
}

// TestSave_RefusesWhatARunCouldNotStart proves a module, which defines no entry
// point, is refused as a bundle: a bundle is read back to be run.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrMain, formerly ErrNoMain (C121, Q348)
func TestSave_RefusesWhatARunCouldNotStart(t *testing.T) {
	_, err := _Built(
		t,
		"lib.star",
	).Save()
	if !errors.Is(
		err,
		spelling.ErrMain,
	) {
		t.Fatalf(
			"got %v, want ErrMain",
			err,
		)
	}
}

// TestRegression_Load proves two bundle defects are refused before any module
// runs.
//
// ClearedLoads: a good bundle with one unit's Loads map cleared. The compiled
// program still lists the load, but the saved map no longer names the target.
// Load returns ErrUnit rather than let the run discover the missing name
// partway through.
//
// NoMain: a bundle whose entry unit defines no entry point. Load then Save
// returns spelling.ErrMain, because Save does not skip the check when the syntax
// tree is nil.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrMain and ErrUnit, formerly ErrNoMain and ErrNoUnit (C121, Q348)
func TestRegression_Load(t *testing.T) {
	t.Run(
		"ClearedLoads",
		func(t *testing.T) {
			bundle := _Handmade(
				t,
				"app.star",
			)

			for _, unit := range bundle.GetUnits() {
				unit.Loads = nil
			}

			raw, err := proto.Marshal(bundle)
			if err != nil {
				t.Fatalf(
					"marshal: %v",
					err,
				)
			}

			_, err = artifact.Load(raw)
			if !errors.Is(
				err,
				artifact.ErrUnit,
			) {
				t.Fatalf(
					"got %v, want ErrUnit",
					err,
				)
			}
		},
	)

	t.Run(
		"NoMain",
		func(t *testing.T) {
			bundle := _Handmade(
				t,
				"app.star",
			)
			bundle.Entry = "lib.star"

			raw, err := proto.Marshal(bundle)
			if err != nil {
				t.Fatalf(
					"marshal: %v",
					err,
				)
			}

			loaded, err := artifact.Load(raw)
			if err != nil {
				t.Fatalf(
					"load: %v",
					err,
				)
			}

			_, err = loaded.Save()
			if !errors.Is(
				err,
				spelling.ErrMain,
			) {
				t.Fatalf(
					"got %v, want ErrMain",
					err,
				)
			}
		},
	)
}

// TestRegression_LoadRefusalText pins the whole text of the refusal for a load
// the saved map does not name. Which unit is reported first is not fixed, so
// the unit name is matched, not spelled.
//
// Revisions:
//   - 2026-11-02 00:31: initial creation, for the C12b extraction
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRegression_LoadRefusalText(t *testing.T) {
	bundle := _Handmade(
		t,
		"app.star",
	)

	for _, unit := range bundle.GetUnits() {
		unit.Loads = nil
	}

	raw, err := proto.Marshal(bundle)
	if err != nil {
		t.Fatalf(
			"marshal: %v",
			err,
		)
	}

	_, err = artifact.Load(raw)

	want := regexp.MustCompile(
		`^\S+\.star loads "\S+\.star", which the bundle does not map: no such unit$`,
	)
	if err == nil || !want.MatchString(err.Error()) {
		t.Fatalf(
			"got %v, want text matching %s",
			err,
			want,
		)
	}
}
