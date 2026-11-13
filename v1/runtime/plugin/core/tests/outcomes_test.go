package tests

import (
	"context"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/spec"
)

// TestOutcomes_ACancelledCallerDoesNotMakeAFalseRecord proves a record comes
// from the handle: with the handle finished and the caller cancelled in the
// same moment, outcomes still answers with the handle's own ok record and
// value. Wait picks between the two at random, so the test is meant to run
// many times:
//
//	go test -race -count=500 -run TestOutcomes_ACancelled ./v1/runtime/plugin/core/
//
// Starting condition: a handle that has returned 7, then a cancelled context.
// Success: one record, ok True, value 7, error and kind None; never an error.
//
// Revisions:
//   - 2026-11-02 16:44: initial creation (C28 fix round 1)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestOutcomes_ACancelledCallerDoesNotMakeAFalseRecord(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(t.Context())
		thread      = _Begun(
			ctx,
			t,
		)
		handle = _Spawned(
			t,
			thread,
			WORKER,
		)
	)

	<-handle.Done()
	cancel()

	got, err := _Call(
		t,
		thread,
		spec.OUTCOMES,
		handle,
	)
	if err != nil {
		t.Fatalf(
			"got %v, want the handle's record",
			err,
		)
	}

	list, ok := got.(*starlark.List)
	if !ok || list.Len() != 1 {
		t.Fatalf(
			"got %v, want a list of one record",
			got,
		)
	}

	want := `{"ok": True, "value": 7, "error": None, "kind": None}`
	if list.Index(0).String() != want {
		t.Fatalf(
			"got %s, want %s",
			list.Index(0),
			want,
		)
	}
}
