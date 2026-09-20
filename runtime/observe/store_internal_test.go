package observe

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

const (
	// MISSING is an id nothing will ever answer to, and LIVE one given to a
	// run that never finishes.
	MISSING = "no-such-run"
	LIVE    = "still-going"

	// HELD is how many finished runs a sweep is asked about at once.
	HELD = 64

	// MOMENT is either side of a deadline, and AGES is long enough that any
	// run looks old - which is the point, since a live run must survive it.
	MOMENT = time.Second
	AGES   = 365 * 24 * time.Hour
)

// TestFind_RefusesAnUnknownId records that an id nothing answers to is an
// error rather than a missing entry.
//
// Internal, because phase 1 has no exported call that reaches _Find. Status,
// Wait and Cancel are later phases and will reach it from outside.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestFind_RefusesAnUnknownId(t *testing.T) {
	store := New()

	_, err := store._Find(MISSING)
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("want ErrUnknown, got %v", err)
	}

	if err.Error() != MISSING+": "+ErrUnknown.Error() {
		t.Fatalf("want the id named in the message, got %q", err)
	}
}

// TestEntry_HoldsNoContext is the structural claim this phase exists to make,
// checked rather than asserted in a comment.
//
// review.md's phase 3 defends a context living on a _Run by arguing that a _Run
// is unreachable once the call that made it returns, and warns that a registry
// storing one past that call silently breaks the argument. Nothing enforces it.
// This does, for this package: no field of an entry is a context, and none is
// anything the scheduler owns.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestEntry_HoldsNoContext(t *testing.T) {
	kind := reflect.TypeOf((*context.Context)(nil)).Elem()
	entry := reflect.TypeOf(_Entry{})

	for i := range entry.NumField() {
		field := entry.Field(i)

		if field.Type == kind {
			t.Fatalf("%s is a context, which this package must not hold", field.Name)
		}

		if field.Type.Implements(kind) {
			t.Fatalf("%s implements context.Context", field.Name)
		}
	}
}

// TestSweep_ForgetsARunNobodyRead is the bound on a store whose runs nobody
// asks about. Without it a finished run is held for the life of the process.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestSweep_ForgetsARunNobodyRead(t *testing.T) {
	store, ended := _Finished(HELD)

	store._Sweep(ended.Add(TTL).Add(-MOMENT))

	if store.Size() != HELD {
		t.Fatalf("want %d runs still held a moment before the deadline, got %d", HELD, store.Size())
	}

	store._Sweep(ended.Add(TTL).Add(MOMENT))

	if store.Size() != 0 {
		t.Fatalf("want every run swept a moment after it, got %d", store.Size())
	}
}

// TestSweep_LeavesALiveRunAlone is phase 2.1, and the reason it was written.
//
// A run still going has never had ended written, so it holds the zero time.
// Any elapsed-time test finds the year 1 older than any age, so a sweep that
// looks at the clock before it looks at completion evicts every live run on the
// first sweep after it starts - while the run carries on. Removing the
// completion guard in _Reap makes this fail.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestSweep_LeavesALiveRunAlone(t *testing.T) {
	store := New()

	store.entries[LIVE] = &_Entry{
		id:   LIVE,
		stop: func() {},
		done: make(chan struct{}),
		into: _NewRecorder(),
	}

	store._Sweep(time.Now().Add(AGES))

	if store.Size() != 1 {
		t.Fatal("want a live run left alone by a sweep an age in the future")
	}

	// The store holding it is half the answer. What the failure would cost is
	// a host no longer able to find a run that is still working, so that is
	// asked separately.
	snap, err := store.Status(LIVE)
	if err != nil {
		t.Fatalf("want a live run still findable, got %v", err)
	}

	if snap.GetStatus() != workflowpb.Status_STATUS_RUNNING {
		t.Fatalf("want it reported running, got %v", snap.GetStatus())
	}
}

// TestSweep_DoesNotDeliverAnEndingItForgot records that sweeping is forgetting,
// not delivering: a swept run's ending reaches nobody.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestSweep_DoesNotDeliverAnEndingItForgot(t *testing.T) {
	store, ended := _Finished(1)

	var id string

	for each := range store.entries {
		id = each
	}

	store._Sweep(ended.Add(TTL).Add(MOMENT))

	_, err := store.Status(id)
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("want a swept run unknown, got %v", err)
	}
}

// _Finished returns a store holding count runs that have already ended, and
// the instant they ended at.
//
// Built rather than run, because what is under test is the sweep and a real
// script would only make the instant harder to name.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func _Finished(count int) (*Store, time.Time) {
	store := New()
	ended := time.Now()

	for range count {
		done := make(chan struct{})
		close(done)

		id := uuid.Must(uuid.NewV7()).String()

		store.entries[id] = &_Entry{
			id:    id,
			stop:  func() {},
			done:  done,
			ended: ended,
			into:  _NewRecorder(),
		}
	}

	return store, ended
}

// TestStatus_SweepsOnARealClock closes a gap the review recorded: every other
// TTL test calls _Sweep with an instant it chose, so all of them would pass
// against a store whose public calls never swept at all.
//
// It cannot wait a day, and shortening TTL would test a different program. So
// it ages the entry instead: a run that ended TTL and a moment ago is one a
// sweep against time.Now() must forget, and one against any other clock will
// not.
//
// Revisions:
//   - 2026-09-20 11:56: initial creation
func TestStatus_SweepsOnARealClock(t *testing.T) {
	store, _ := _Finished(1)

	_Age(store, time.Now().Add(-TTL).Add(-MOMENT))

	var id string

	for each := range store.entries {
		id = each
	}

	_, err := store.Status(id)
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("want Status to have swept a run older than the ttl, got %v", err)
	}

	if store.Size() != 0 {
		t.Fatalf("want the store emptied by a real clock, got %d", store.Size())
	}
}

// TestStart_SweepsOnARealClock is the same for the other call that sweeps.
//
// A host that starts runs and never asks about them is exactly the case the
// ttl exists for, so Start sweeping is not decoration.
//
// Revisions:
//   - 2026-09-20 11:56: initial creation
func TestStart_SweepsOnARealClock(t *testing.T) {
	store, _ := _Finished(HELD)

	_Age(store, time.Now().Add(-TTL).Add(-MOMENT))

	store.Start(t.Context(), nil)

	if store.Size() != 1 {
		t.Fatalf("want only the new run left, got %d", store.Size())
	}
}

// _Age moves every run in a store to have ended at the given instant.
//
// Revisions:
//   - 2026-09-20 11:56: initial creation
func _Age(store *Store, when time.Time) {
	for _, entry := range store.entries {
		entry.ended = when
	}
}

// TestStart_SurvivesSomethingThatIsNotAnArtifact records that the goroutine is
// guarded.
//
// Found by writing the sweep test above: passing nil took the whole test binary
// down, because a panic on a goroutine cannot be recovered from outside it.
// Invoke guards the script's own evaluation, and everything before it was
// unguarded - which is the failure mode lark-runtime phase 9 exists for,
// arriving through a door phase 1 opened.
//
// Revisions:
//   - 2026-09-20 11:56: initial creation
func TestStart_SurvivesSomethingThatIsNotAnArtifact(t *testing.T) {
	store := New()

	id := store.Start(t.Context(), nil)

	_, err := store.Wait(t.Context(), id)
	if err == nil {
		t.Fatal("want the run to have failed")
	}

	status, _ := store.entries[id]._Ending()
	if status != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want it reported failed, got %v", status)
	}
}

// TestStatus_ForgetsOnlyBySweeping records that the sweep is now the only thing
// that removes anything.
//
// A run read and then left is still there; a run read and then aged past the
// ttl is gone. Before 2026-09-20 12:04 the first read removed it, so this pair
// could not both have been true.
//
// Revisions:
//   - 2026-09-20 12:05: initial creation
func TestStatus_ForgetsOnlyBySweeping(t *testing.T) {
	store, _ := _Finished(1)

	var id string

	for each := range store.entries {
		id = each
	}

	_, err := store.Status(id)
	if err != nil {
		t.Fatal(err)
	}

	if store.Size() != 1 {
		t.Fatalf("want a read run still held, got %d", store.Size())
	}

	_Age(store, time.Now().Add(-TTL).Add(-MOMENT))

	_, err = store.Status(id)
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("want the sweep to have forgotten it, got %v", err)
	}
}
