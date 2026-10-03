package artifact_test

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thebagchi/lark/v1/runtime/artifact"
)

// LOADED_SUM is what app.star returns: double(2) from lib.star and triple(3)
// from helper.star.
const LOADED_SUM = 13

// TestLoad_RunsWhatSaveWrote proves a bundle is a program: saved, read back
// with nothing but the bytes, and run to the answer the compiled artifact gives,
// the modules it loads included.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
func TestLoad_RunsWhatSaveWrote(t *testing.T) {
	saved, err := _Built(t, SAVE_FIXTURE).Save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := artifact.Load(saved)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	value, err := artifact.Run(t.Context(), loaded)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if got := _Number(t, value); got != LOADED_SUM {
		t.Fatalf("got %d, want %d", got, LOADED_SUM)
	}
}

// TestLoad_RefusesABundleMissingAUnit proves a bundle that does not hold a
// module it loads is refused when it is read, rather than partway through a
// run.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
func TestLoad_RefusesABundleMissingAUnit(t *testing.T) {
	bundle := _Bundle(t, SAVE_FIXTURE)
	bundle.Units = bundle.GetUnits()[1:]

	short, err := proto.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	_, err = artifact.Load(short)
	if !errors.Is(err, artifact.ERR_NO_UNIT) {
		t.Fatalf("got %v, want ERR_NO_UNIT", err)
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
	if !errors.Is(err, artifact.ERR_NO_UNIT) {
		t.Fatalf("an empty bundle gave %v, want ERR_NO_UNIT", err)
	}
}

// TestSave_RefusesWhatARunCouldNotStart proves a module, which defines no entry
// point, is refused as a bundle: a bundle is read back to be run.
//
// Revisions:
//   - 2026-10-03 00:27: initial creation
func TestSave_RefusesWhatARunCouldNotStart(t *testing.T) {
	_, err := _Built(t, LIB_FIXTURE).Save()
	if !errors.Is(err, artifact.ERR_NO_MAIN) {
		t.Fatalf("got %v, want ERR_NO_MAIN", err)
	}
}
