package observe_test

import (
	"context"
	"errors"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/artifact"
	"github.com/thebagchi/lark/runtime/observe"
	"github.com/thebagchi/lark/runtime/scheduler"
)

const (
	SLOW    = "testdata/slow.star"
	QUICK   = "testdata/quick.star"
	FAILING = "testdata/failing.star"

	// RESULT is what the quick script returns, and MISSING an id nothing will
	// ever answer to.
	RESULT  = "7"
	MISSING = "no-such-run"

	// PATIENCE is how long a test waits for something that should be
	// immediate. Generous, because a loaded machine is not a bug.
	PATIENCE = 5 * time.Second

	// SETTLE is long enough that a run which was wrongly cancelled has ended,
	// and far short of the thirty seconds a live one still has to sleep.
	SETTLE = 250 * time.Millisecond

	// POLLS is how many times a caller asks in a row, and POLLERS how many ask
	// at one moment, over ROUNDS stagings of that moment.
	POLLS   = 8
	POLLERS = 64
	ROUNDS  = 50

	// PROMPT is how long a start may take and still be said not to have waited
	// for a thirty second script. Generous, because a loaded machine is not a
	// bug and the thing being measured differs by four orders of magnitude.
	PROMPT = time.Second

	// RACERS is how many goroutines start runs at one moment.
	RACERS = 32
)

// CANONICAL is the shape of a version 7 UUID, with the version nibble and the
// variant bits pinned to what RFC 9562 says one carries.
var CANONICAL = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

// _Compile builds an artifact from a script on disk.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
func _Compile(t *testing.T, path string) *artifact.Artifact {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	built, err := artifact.NewCompiler().Compile(path, src)
	if err != nil {
		t.Fatal(err)
	}

	return built
}

// TestStart_ReturnsBeforeTheRunDoes is §3.4: a caller is given an id while the
// script is still going, rather than a result when it is over.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
func TestStart_ReturnsBeforeTheRunDoes(t *testing.T) {
	store := observe.New()

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	begun := time.Now()
	id := store.Start(ctx, _Compile(t, SLOW))
	taken := time.Since(begun)

	if taken > PROMPT {
		t.Fatalf("starting a 30 second script took %s", taken)
	}

	if store.Size() != 1 {
		t.Fatalf("want the run held, got %d entries", store.Size())
	}

	if id == "" {
		t.Fatal("want an id")
	}
}

// TestStart_GivesAVersionSevenUuid records what an id is, because a counter
// would pass every other test in this file and re-issue its first id after a
// restart.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
func TestStart_GivesAVersionSevenUuid(t *testing.T) {
	store := observe.New()

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	id := store.Start(ctx, _Compile(t, QUICK))

	if !CANONICAL.MatchString(id) {
		t.Fatalf("%q is not a canonical version 7 uuid", id)
	}
}

// TestStart_GivesConcurrentStartsDistinctIds runs the store the way a host
// would, since nothing serialises callers.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
func TestStart_GivesConcurrentStartsDistinctIds(t *testing.T) {
	store := observe.New()
	built := _Compile(t, QUICK)

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	ids := make(chan string, RACERS)

	for range RACERS {
		go func() {
			ids <- store.Start(ctx, built)
		}()
	}

	seen := make(map[string]bool, RACERS)

	for range RACERS {
		id := <-ids

		if seen[id] {
			t.Fatalf("want distinct ids, got %s twice", id)
		}

		seen[id] = true
	}

	if store.Size() != RACERS {
		t.Fatalf("want %d runs held, got %d", RACERS, store.Size())
	}
}

// TestStatus_ReportsRunningWithoutForgetting is the polling a user interface
// does over and over: asking about a live run is free of consequence.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestStatus_ReportsRunningWithoutForgetting(t *testing.T) {
	store := observe.New()

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	id := store.Start(ctx, _Compile(t, SLOW))

	for i := range POLLS {
		snap, err := store.Status(id)
		if err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}

		if snap.GetStatus() != workflowpb.Status_STATUS_RUNNING {
			t.Fatalf("poll %d: want running, got %v", i, snap.GetStatus())
		}
	}

	if store.Size() != 1 {
		t.Fatalf("want the run still held, got %d entries", store.Size())
	}
}

// TestStatus_KeepsARunItHasReported is the contract from 2026-09-20 12:04:
// reading an ending does not consume it.
//
// This test previously asserted the opposite, and is inverted rather than
// replaced - it recorded a real property of a real design, and the property it
// records now is that design's reversal.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as TestStatus_ForgetsARunOnceItsEndingIsRead
//   - 2026-09-20 12:04: inverted, Status having stopped forgetting
func TestStatus_KeepsARunItHasReported(t *testing.T) {
	store := observe.New()
	id := store.Start(t.Context(), _Compile(t, QUICK))

	_, err := store.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Status(id)
	if err != nil {
		t.Fatal(err)
	}

	if first.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want succeeded, got %v", first.GetStatus())
	}

	if store.Size() != 1 {
		t.Fatalf("want the run still held, got %d entries", store.Size())
	}

	second, err := store.Status(id)
	if err != nil {
		t.Fatalf("want a second read answered, got %v", err)
	}

	if second.GetStatus() != first.GetStatus() {
		t.Fatalf("want the same answer twice, got %v then %v",
			first.GetStatus(), second.GetStatus())
	}
}

// TestStatus_CarriesAFailedEnding records that an ending says which one it was.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestStatus_CarriesAFailedEnding(t *testing.T) {
	store := observe.New()
	id := store.Start(t.Context(), _Compile(t, FAILING))

	_, err := store.Wait(t.Context(), id)
	if err == nil {
		t.Fatal("want the assertion reported")
	}

	snap, err := store.Status(id)
	if err != nil {
		t.Fatal(err)
	}

	if snap.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want failed, got %v", snap.GetStatus())
	}
}

// TestStatus_DeliversAnEndingToEveryCaller is the reversal of §3.9, decided
// 2026-09-20 12:04: two interfaces watching one run are both answered.
//
// The gate and the rounds are kept from the test this replaces, which demanded
// exactly one caller be told. They were sized from a measurement and the reason
// for them survives the reversal: without a gate the first goroutine finishes
// before the last one starts, so the callers never overlap, and a test that
// cannot make them overlap is not a test of concurrent readers whichever answer
// it expects.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as
//     TestStatus_DeliversAnEndingToExactlyOneCaller
//   - 2026-09-20 12:04: inverted; every caller is told, where one was
func TestStatus_DeliversAnEndingToEveryCaller(t *testing.T) {
	built := _Compile(t, QUICK)

	for round := range ROUNDS {
		store := observe.New()
		id := store.Start(t.Context(), built)

		_, err := store.Wait(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}

		seen := _Poll(t, store, id)
		if seen != POLLERS {
			t.Fatalf("round %d: want all %d callers told the ending, got %d",
				round, POLLERS, seen)
		}
	}
}

// _Poll releases POLLERS callers at one moment and counts how many were told
// how the run ended.
//
// Every caller waits on one channel and is released by closing it. Without a
// gate the first goroutine finishes before the last one starts, so the callers
// never overlap and nothing here is a test of anything.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func _Poll(t *testing.T, store *observe.Store, id string) int {
	t.Helper()

	var seen atomic.Int64

	var group sync.WaitGroup

	gate := make(chan struct{})

	for range POLLERS {
		group.Add(1)

		go func() {
			defer group.Done()

			<-gate

			snap, err := store.Status(id)
			if err != nil {
				return
			}

			if snap.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
				t.Errorf("want the ending, got %v", snap.GetStatus())
			}

			seen.Add(1)
		}()
	}

	close(gate)
	group.Wait()

	return int(seen.Load())
}

// TestWait_IsIdempotentAndDoesNotForget is the asymmetry with Status, checked
// rather than left in a doc comment.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestWait_IsIdempotentAndDoesNotForget(t *testing.T) {
	store := observe.New()
	id := store.Start(t.Context(), _Compile(t, QUICK))

	first, err := store.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	second, err := store.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	if first.String() != RESULT || second.String() != RESULT {
		t.Fatalf("want %s twice, got %s and %s", RESULT, first, second)
	}

	if store.Size() != 1 {
		t.Fatalf("want waiting not to forget, got %d entries", store.Size())
	}
}

// TestWait_CarriesTheRunsOwnFailure records that a script's failure reaches
// through this layer unwrapped, so a host can name it.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestWait_CarriesTheRunsOwnFailure(t *testing.T) {
	store := observe.New()
	id := store.Start(t.Context(), _Compile(t, FAILING))

	_, err := store.Wait(t.Context(), id)
	if !errors.Is(err, scheduler.ErrAssert) {
		t.Fatalf("want an assertion, got %v", err)
	}
}

// TestWait_LetsACallerGiveUpWithoutStoppingTheRun is the reason Wait takes a
// context at all, and the check that the two contexts are not confused.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestWait_LetsACallerGiveUpWithoutStoppingTheRun(t *testing.T) {
	store := observe.New()

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	id := store.Start(ctx, _Compile(t, SLOW))

	patience, done := context.WithCancel(t.Context())
	done()

	_, err := store.Wait(patience, id)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want the waiter's own cancellation, got %v", err)
	}

	// Asking Status here would prove nothing. A cancelled evaluation stops at
	// its next instruction, so a run killed by this very call still reports
	// running for a moment afterwards - the check would pass against the bug
	// it exists to catch. Waiting again with a budget asks the question at a
	// moment when the answer has settled: a run still sleeping cannot finish
	// inside SETTLE, and a run that was killed finishes at once.
	second, expire := context.WithTimeout(t.Context(), SETTLE)
	defer expire()

	_, err = store.Wait(second, id)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want the run still sleeping, got %v", err)
	}
}

// TestCancel_ReachesASpawnedSleep is §3.11, and the part worth proving: the
// store holds only a cancel function, yet cancelling by id still reaches a
// spawned thread parked in sleep.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestCancel_ReachesASpawnedSleep(t *testing.T) {
	store := observe.New()
	id := store.Start(t.Context(), _Compile(t, SLOW))

	ended := make(chan error, 1)

	go func() {
		_, err := store.Wait(t.Context(), id)
		ended <- err
	}()

	err := store.Cancel(id)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-ended:
		if !errors.Is(got, context.Canceled) {
			t.Fatalf("want a cancellation, got %v", got)
		}
	case <-time.After(PATIENCE):
		t.Fatal("want a cancel to end a 30 second sleep, not to wait it out")
	}
}

// TestCancel_IsIdempotentAndForgivesAFinishedRun records the two calls a host
// racing a run to its end would otherwise have to guard.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestCancel_IsIdempotentAndForgivesAFinishedRun(t *testing.T) {
	store := observe.New()
	id := store.Start(t.Context(), _Compile(t, QUICK))

	_, err := store.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	for i := range POLLS {
		err = store.Cancel(id)
		if err != nil {
			t.Fatalf("cancel %d: %v", i, err)
		}
	}
}

// TestStore_RefusesAnUnknownIdEverywhere records that every call that takes an
// id refuses one nothing answers to.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestStore_RefusesAnUnknownIdEverywhere(t *testing.T) {
	store := observe.New()

	_, err := store.Status(MISSING)
	if !errors.Is(err, observe.ErrUnknown) {
		t.Fatalf("status: want ErrUnknown, got %v", err)
	}

	_, err = store.Wait(t.Context(), MISSING)
	if !errors.Is(err, observe.ErrUnknown) {
		t.Fatalf("wait: want ErrUnknown, got %v", err)
	}

	err = store.Cancel(MISSING)
	if !errors.Is(err, observe.ErrUnknown) {
		t.Fatalf("cancel: want ErrUnknown, got %v", err)
	}
}
