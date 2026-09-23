package observe_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/artifact"
	"github.com/thebagchi/lark/runtime/observe"
	"github.com/thebagchi/lark/runtime/plugin/core"
)

const (
	SLOW    = "testdata/slow.star"
	QUICK   = "testdata/quick.star"
	FAILING = "testdata/failing.star"

	// RESULT is what the quick script returns.
	RESULT = "7"

	// PATIENCE is how long a test waits for something that should be
	// immediate. Generous, because a loaded machine is not a bug.
	PATIENCE = 5 * time.Second

	// SETTLE is long enough that a run which was wrongly stopped has ended,
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

// TestStart_ReturnsBeforeTheRunDoes is what starting a run is for: a caller is
// handed the run while the script is still going, rather than a result when it
// is over.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
//   - 2026-09-23 23:28: the run is what comes back, so there is no store to
//     count entries in
func TestStart_ReturnsBeforeTheRunDoes(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	begun := time.Now()
	run := observe.Start(ctx, _Compile(t, SLOW))
	taken := time.Since(begun)

	if taken > PROMPT {
		t.Fatalf("starting a 30 second script took %s", taken)
	}

	if run == nil {
		t.Fatal("want the run")
	}
}

// TestStatus_ReportsRunning checks that asking does not consume, however often
// a caller asks.
//
// Revisions:
//   - 2026-09-20 01:37: initial creation
//   - 2026-09-23 23:28: nothing forgets, so this checks only the answer
func TestStatus_ReportsRunning(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	run := observe.Start(ctx, _Compile(t, SLOW))

	for i := range POLLS {
		snap := run.Status()

		if snap.GetStatus() != workflowpb.Status_STATUS_RUNNING {
			t.Fatalf("poll %d: want running, got %v", i, snap.GetStatus())
		}
	}
}

// TestStatus_KeepsAnsweringAfterTheRunEnded is the property that used to
// depend on a sweep not having happened yet, and now depends on nothing: a
// caller holding a run is answered for as long as it holds it.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as
//     TestStatus_ForgetsARunOnceItsEndingIsRead
//   - 2026-09-20 12:04: inverted, Status having stopped forgetting
//   - 2026-09-23 23:28: there is nothing left that could forget
func TestStatus_KeepsAnsweringAfterTheRunEnded(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, QUICK))

	_, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	first := run.Status()
	if first.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want succeeded, got %v", first.GetStatus())
	}

	second := run.Status()
	if second.GetStatus() != first.GetStatus() {
		t.Fatalf("want the same answer twice, got %v then %v",
			first.GetStatus(), second.GetStatus())
	}
}

// TestStatus_CarriesAFailedEnding records that an ending says which one it
// was.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestStatus_CarriesAFailedEnding(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, FAILING))

	_, err := run.Wait()
	if err == nil {
		t.Fatal("want the assertion reported")
	}

	if run.Status().GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want failed, got %v", run.Status().GetStatus())
	}
}

// TestStatus_AnswersEveryCallerAtOnce checks that two interfaces watching one
// run are both told.
//
// The gate and the rounds are sized from a measurement: without a gate the
// first goroutine finishes before the last one starts, so the callers never
// overlap, and a test that cannot make them overlap is not a test of
// concurrent readers.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as
//     TestStatus_DeliversAnEndingToExactlyOneCaller
//   - 2026-09-20 12:04: inverted; every caller is told, where one was
//   - 2026-09-23 23:28: asks the run rather than a store
func TestStatus_AnswersEveryCallerAtOnce(t *testing.T) {
	built := _Compile(t, QUICK)

	for round := range ROUNDS {
		run := observe.Start(t.Context(), built)

		_, err := run.Wait()
		if err != nil {
			t.Fatal(err)
		}

		seen := _Poll(t, run)
		if seen != POLLERS {
			t.Fatalf("round %d: want all %d callers told the ending, got %d",
				round, POLLERS, seen)
		}
	}
}

// _Poll releases POLLERS callers at one moment and counts how many were told
// how the run ended.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func _Poll(t *testing.T, run *observe.Execution) int {
	t.Helper()

	var (
		seen  atomic.Int64
		group sync.WaitGroup
	)

	gate := make(chan struct{})

	for range POLLERS {
		group.Add(1)

		go func() {
			defer group.Done()

			<-gate

			snap := run.Status()

			if snap.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
				t.Errorf("want the ending, got %v", snap.GetStatus())

				return
			}

			seen.Add(1)
		}()
	}

	close(gate)
	group.Wait()

	return int(seen.Load())
}

// TestWait_IsIdempotent checks that every caller that waits is told, and told
// the same thing.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
//   - 2026-09-23 23:28: nothing forgets, so there is no size to check
func TestWait_IsIdempotent(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, QUICK))

	first, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	second, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if first.String() != RESULT || second.String() != RESULT {
		t.Fatalf("want %s twice, got %s and %s", RESULT, first, second)
	}
}

// TestWait_CarriesTheRunsOwnFailure records that a script's failure reaches
// through this layer unwrapped, so a host can name it.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func TestWait_CarriesTheRunsOwnFailure(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, FAILING))

	_, err := run.Wait()
	if !errors.Is(err, core.ErrAssert) {
		t.Fatalf("want an assertion, got %v", err)
	}
}

// TestDone_LetsACallerGiveUpWithoutStoppingTheRun is what Done is for, and the
// check that giving up is not stopping.
//
// Wait no longer takes a context, because a caller that would rather not block
// has this instead - and a context on Wait was two contexts in one call, which
// is what this test was originally written to keep apart.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as
//     TestWait_LetsACallerGiveUpWithoutStoppingTheRun
//   - 2026-09-23 23:28: gives up by not waiting, rather than by cancelling a
//     waiter's own context
func TestDone_LetsACallerGiveUpWithoutStoppingTheRun(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	run := observe.Start(ctx, _Compile(t, SLOW))

	select {
	case <-run.Done():
		t.Fatal("want the run still going")
	default:
	}

	// A run still sleeping cannot finish inside SETTLE, and a run that was
	// stopped by the giving up finishes at once - so waiting this long asks
	// the question at a moment when the answer has settled.
	select {
	case <-run.Done():
		t.Fatal("giving up stopped the run")
	case <-time.After(SETTLE):
	}
}

// TestStop_ReachesASpawnedSleep is the part worth proving: a run holds only a
// cancel function, yet stopping it still reaches a spawned thread parked in
// sleep.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as TestCancel_ReachesASpawnedSleep
//   - 2026-09-23 23:28: stops the run it holds
func TestStop_ReachesASpawnedSleep(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, SLOW))

	run.Stop()

	select {
	case <-run.Done():
	case <-time.After(PATIENCE):
		t.Fatal("a stopped run did not end")
	}

	_, err := run.Wait()
	if err == nil {
		t.Fatal("want a stopped run to say so")
	}

	if run.Status().GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf("want cancelled, got %v", run.Status().GetStatus())
	}
}

// TestStop_IsIdempotentAndForgivesAFinishedRun checks that stopping twice, and
// stopping something already over, are both quiet.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as
//     TestCancel_IsIdempotentAndForgivesAFinishedRun
//   - 2026-09-23 23:28: stops the run it holds
func TestStop_IsIdempotentAndForgivesAFinishedRun(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, QUICK))

	got, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	run.Stop()
	run.Stop()

	if run.Status().GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("stopping a finished run changed it to %v", run.Status().GetStatus())
	}

	again, err := run.Wait()
	if err != nil || again.String() != got.String() {
		t.Fatalf("after stopping got %v, %v; want the value unchanged", again, err)
	}
}
