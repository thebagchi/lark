package artifact_test

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thebagchi/lark/v1/runtime/artifact"
)

// LOADED_SUM is what app.star returns: double(2) from lib.star and triple(3)
// from helper.star.
const (
	LOADED_SUM = 13
)

// TestLoad_RunsWhatSaveWrote proves a bundle is a program: saved, read back
// with nothing but the bytes, and run to the answer the compiled artifact gives,
// the modules it loads included.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
func TestLoad_RunsWhatSaveWrote(t *testing.T) {
	saved, err := _Built(
		t,
		SAVE_FIXTURE,
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
func TestLoad_RefusesABundleMissingAUnit(t *testing.T) {
	bundle := _Bundle(
		t,
		SAVE_FIXTURE,
	)
	bundle.Units = bundle.GetUnits()[1:]

	short, err := proto.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	_, err = artifact.Load(short)
	if !errors.Is(
		err,
		artifact.ErrNoUnit,
	) {
		t.Fatalf(
			"got %v, want ErrNoUnit",
			err,
		)
	}
}

// TestLoad_RefusesWhatIsNotABundle proves bytes that are not a bundle are
// refused, and so is an empty one, which names no entry.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
func TestLoad_RefusesWhatIsNotABundle(t *testing.T) {
	_, err := artifact.Load([]byte("not a bundle"))
	if err == nil {
		t.Fatal("want bytes that are not a bundle refused")
	}

	_, err = artifact.Load(nil)
	if !errors.Is(
		err,
		artifact.ErrNoUnit,
	) {
		t.Fatalf(
			"an empty bundle gave %v, want ErrNoUnit",
			err,
		)
	}
}

// TestSave_RefusesWhatARunCouldNotStart proves a module, which defines no entry
// point, is refused as a bundle: a bundle is read back to be run.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
func TestSave_RefusesWhatARunCouldNotStart(t *testing.T) {
	_, err := _Built(
		t,
		LIB_FIXTURE,
	).Save()
	if !errors.Is(
		err,
		artifact.ErrNoMain,
	) {
		t.Fatalf(
			"got %v, want ErrNoMain",
			err,
		)
	}
}

// TestRegression_Load proves two bundle defects are refused before any module
// runs.
//
// ClearedLoads: a good bundle with one unit's Loads map cleared. The compiled
// program still lists the load, but the saved map no longer names the target.
// Load returns ErrNoUnit rather than let the run discover the missing name
// partway through.
//
// NoMain: a bundle whose entry unit defines no entry point. Load then Save
// returns ErrNoMain, because Save does not skip the check when the syntax
// tree is nil.
//
// Revisions:
//   - 2026-10-24 16:05: initial creation
func TestRegression_Load(t *testing.T) {
	t.Run(
		"ClearedLoads",
		func(t *testing.T) {
			bundle := _Bundle(
				t,
				SAVE_FIXTURE,
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
				artifact.ErrNoUnit,
			) {
				t.Fatalf(
					"got %v, want ErrNoUnit",
					err,
				)
			}
		},
	)

	t.Run(
		"NoMain",
		func(t *testing.T) {
			bundle := _Bundle(
				t,
				SAVE_FIXTURE,
			)
			bundle.Entry = LIB_FIXTURE

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
				artifact.ErrNoMain,
			) {
				t.Fatalf(
					"got %v, want ErrNoMain",
					err,
				)
			}
		},
	)
}
