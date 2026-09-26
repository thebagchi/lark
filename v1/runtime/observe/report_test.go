package observe_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/observe"
)

const (
	BRANCHING   = "testdata/branching.star"
	ONEFAILS    = "testdata/onefails.star"
	RETRYING    = "testdata/retrying.star"
	REPEATING   = "testdata/repeating.star"
	BOUNDED     = "testdata/bounded.star"
	RETRYSLOW   = "testdata/retryslow.star"
	SLOWSIBLING = "testdata/slowsibling.star"

	// BREAKS is the text the failing sample asserts with.
	BREAKS = "this one breaks"

	// SPINE is the entry point's thread, and UNREACHED a function the
	// branching script declares and never calls.
	SPINE     = "thread_0"
	UNREACHED = "unreached"

	// ENTRY is the function a run starts at.
	ENTRY = "main"

	// TRIES is how many attempts the retrying and repeating scripts make.
	TRIES = 3

	// TICK is how often the polling watcher looks, chosen well under the 50
	// milliseconds each repeated step sleeps for.
	TICK = 5 * time.Millisecond
)

// _Ran starts a script, waits for it, and returns its final report.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
//   - 2026-09-21 08:09: logs how the run ended rather than discarding it
//   - 2026-09-23 23:28: asks the run it started, there being no store
func _Ran(t *testing.T, path string, opts ...observe.Option) *workflowpb.Workflow {
	t.Helper()

	run := observe.Start(t.Context(), _Compile(t, path), opts...)

	_, err := run.Wait()
	if err != nil {
		// How the run ended is what the snapshot below reports, and several
		// of these scripts end badly on purpose.
		t.Logf("%s ended with: %v", path, err)
	}

	return run.Status()
}

// _Node finds one function's node, wherever it is, and says which thread it was
// on.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func _Node(snap *workflowpb.Workflow, name string) (*workflowpb.Node, string) {
	for _, lane := range snap.GetThreads() {
		for _, node := range lane.GetLive().GetNodes() {
			if node.GetFunction() == name {
				return node, lane.GetId()
			}
		}
	}

	return nil, ""
}

// TestReport_NamesEveryFunctionThatRan is §7.2: a spawned function appears, on
// a thread of its own, and the entry point is on the spine.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_NamesEveryFunctionThatRan(t *testing.T) {
	snap := _Ran(t, BRANCHING)

	entry, lane := _Node(snap, "main")
	if entry == nil {
		t.Fatal("want the entry point reported")
	}

	if lane != SPINE {
		t.Fatalf("want main on the spine, got thread %s", lane)
	}

	lanes := make(map[string]bool)

	for _, name := range []string{"alpha", "beta"} {
		node, on := _Node(snap, name)
		if node == nil {
			t.Fatalf("want %s reported", name)
		}

		if on == SPINE {
			t.Fatalf("want %s on its own thread, got the spine", name)
		}

		if lanes[on] {
			t.Fatalf("want alpha and beta on different threads, both got %s", on)
		}

		lanes[on] = true

		if node.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
			t.Fatalf("want %s succeeded, got %v", name, node.GetStatus())
		}
	}
}

// TestReport_LeavesOutWhatNothingCalled is the default chosen on 2026-09-20
// 01:05: without a graph, a report says what the run did.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_LeavesOutWhatNothingCalled(t *testing.T) {
	node, _ := _Node(_Ran(t, BRANCHING), UNREACHED)

	if node != nil {
		t.Fatalf("want a function nothing called left out, got %v", node.GetStatus())
	}
}

// TestReport_FailsOnlyWhatFailed records that one failing thread does not make
// its sibling look failed.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_FailsOnlyWhatFailed(t *testing.T) {
	snap := _Ran(t, ONEFAILS)

	if snap.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want the run failed, got %v", snap.GetStatus())
	}

	bad, _ := _Node(snap, "bad")
	if bad == nil || bad.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want bad failed, got %v", bad)
	}

	good, _ := _Node(snap, "good")
	if good == nil || good.GetStatus() == workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want good not failed, got %v", good)
	}
}

// TestReport_CarriesTheAttemptARetryReached is §7.3 through a retry: one node,
// carrying the attempt that succeeded.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_CarriesTheAttemptARetryReached(t *testing.T) {
	snap := _Ran(t, RETRYING)

	node, _ := _Node(snap, "flaky")
	if node == nil {
		t.Fatal("want the retried function reported")
	}

	if node.GetAttempt() != TRIES {
		t.Fatalf("want attempt %d, got %d", TRIES, node.GetAttempt())
	}

	if node.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want it succeeded in the end, got %v", node.GetStatus())
	}

	if _Count(snap, "flaky") != 1 {
		t.Fatalf("want one node for three attempts, got %d", _Count(snap, "flaky"))
	}
}

// TestReport_GivesAPlainCallNoAttempt records that zero means not inside a
// repeat or a retry, including for a timeout, which makes one call rather than
// attempts.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_GivesAPlainCallNoAttempt(t *testing.T) {
	spawned, _ := _Node(_Ran(t, BRANCHING), "alpha")
	if spawned.GetAttempt() != 0 {
		t.Fatalf("want a spawned function to carry no attempt, got %d", spawned.GetAttempt())
	}

	bounded, _ := _Node(_Ran(t, BOUNDED), "swift")
	if bounded == nil {
		t.Fatal("want the timed function reported")
	}

	if bounded.GetAttempt() != 0 {
		t.Fatalf("want a timeout to carry no attempt, got %d", bounded.GetAttempt())
	}
}

// _Count is how many nodes name this function, across every thread.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func _Count(snap *workflowpb.Workflow, name string) int {
	found := 0

	for _, lane := range snap.GetThreads() {
		for _, node := range lane.GetLive().GetNodes() {
			if node.GetFunction() == name {
				found++
			}
		}
	}

	return found
}

// TestReport_NeverShowsACaughtFailure is the row phase 6 exists for, and the
// one that would be got wrong by writing the obvious thing.
//
// A retry's first two attempts fail. Ending every attempt would report each as
// STATUS_FAILED, so a user interface polling in between would show a red
// function about to be fine, and a host watching for failures would see two
// that never happened. Only the wrapper ends, once.
//
// It has to poll throughout rather than at the end: a test that only looks
// afterwards passes against exactly the implementation this rules out.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_NeverShowsACaughtFailure(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, RETRYSLOW))

	seen := 0
	attempts := make(map[int32]bool)

	for {
		snap := run.Status()

		node, _ := _Node(snap, "flaky")
		if node != nil {
			seen++
			attempts[node.GetAttempt()] = true

			if node.GetStatus() == workflowpb.Status_STATUS_FAILED {
				t.Fatalf("attempt %d was reported failed while the retry was still going",
					node.GetAttempt())
			}
		}

		if snap.GetStatus() != workflowpb.Status_STATUS_RUNNING {
			break
		}

		time.Sleep(TICK)
	}

	if seen < TRIES {
		t.Fatalf("want the retry polled while it ran, only saw it %d times", seen)
	}

	if len(attempts) < TRIES {
		t.Fatalf("want every attempt observed, saw %d distinct", len(attempts))
	}
}

// TestReport_CountsEveryRepeat records that repeat advances one node rather
// than adding one per call.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_CountsEveryRepeat(t *testing.T) {
	snap := _Ran(t, REPEATING)

	node, _ := _Node(snap, "step")
	if node == nil {
		t.Fatal("want the repeated function reported")
	}

	if node.GetAttempt() != TRIES {
		t.Fatalf("want attempt %d, got %d", TRIES, node.GetAttempt())
	}

	if _Count(snap, "step") != 1 {
		t.Fatalf("want one node for three calls, got %d", _Count(snap, "step"))
	}
}

// TestReport_CancelledIsNotFailed is sub-phase 3.1: a run a caller stopped did
// not break, and the schema can finally say so.
//
// Revisions:
//   - 2026-09-20 11:34: initial creation
func TestReport_CancelledIsNotFailed(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, SLOW))

	run.Stop()

	_, err := run.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want the cancellation, got %v", err)
	}

	snap := run.Status()

	if snap.GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf("want cancelled, got %v", snap.GetStatus())
	}
}

// TestReport_AFailureIsStillAFailure is the other half: adding a value must not
// make ordinary failures start reporting as something else.
//
// Revisions:
//   - 2026-09-20 11:34: initial creation
func TestReport_AFailureIsStillAFailure(t *testing.T) {
	snap := _Ran(t, ONEFAILS)

	if snap.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want the run failed, got %v", snap.GetStatus())
	}
}

// TestReport_ASiblingStoppedByFailFastIsCancelled is why this is worth having
// beyond a host that called Cancel.
//
// This runtime is fail-fast, so every failing run stops threads that were doing
// nothing wrong. Before this they reported failed, and one broken script looked
// like several.
//
// Revisions:
//   - 2026-09-20 11:34: initial creation
func TestReport_ASiblingStoppedByFailFastIsCancelled(t *testing.T) {
	snap := _Ran(t, SLOWSIBLING)

	bad, _ := _Node(snap, "bad")
	if bad == nil || bad.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want the function that asserted to be failed, got %v", bad)
	}

	slow, _ := _Node(snap, "patient")
	if slow == nil {
		t.Fatal("want the cancelled sibling reported")
	}

	if slow.GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf("want the sibling cancelled rather than failed, got %v", slow.GetStatus())
	}
}

// TestReport_AFailureCarriesItsMessage is sub-phase 3.2: a status says a run
// failed, and this says what went wrong.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
func TestReport_AFailureCarriesItsMessage(t *testing.T) {
	snap := _Ran(t, ONEFAILS)

	if snap.GetCause() == nil {
		t.Fatal("want the run to say what ended it")
	}

	if !strings.Contains(snap.GetCause().GetFailure(), BREAKS) {
		t.Fatalf("want the assertion's own words, got %q", snap.GetCause().GetFailure())
	}

	bad, _ := _Node(snap, "bad")
	if !strings.Contains(bad.GetFailure(), BREAKS) {
		t.Fatalf("want the failing node to carry its message, got %q", bad.GetFailure())
	}

	good, _ := _Node(snap, "good")
	if good.GetFailure() != "" {
		t.Fatalf("want a node that did not fail to carry nothing, got %q", good.GetFailure())
	}
}

// TestReport_ACancellationCarriesNoMessage is the other half of the rule, and
// the reason the field is called failure rather than message.
//
// A cancellation's text says it was cancelled, which the status says. Filling
// it in would undo sub-phase 3.1, whose whole point is that a reader does not
// have to read prose to learn a thread was stopped.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
func TestReport_ACancellationCarriesNoMessage(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, SLOW))

	run.Stop()

	_, err := run.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want the cancellation, got %v", err)
	}

	snap := run.Status()

	if snap.GetCause() != nil {
		t.Fatalf("want a cancelled run to have no cause, got %v", snap.GetCause())
	}

	// And the sibling stopped by a failure, which is the common case.
	stopped := _Ran(t, SLOWSIBLING)

	patient, _ := _Node(stopped, "patient")
	if patient.GetFailure() != "" {
		t.Fatalf("want a stopped sibling to carry nothing, got %q", patient.GetFailure())
	}
}

// TestReport_SuccessCarriesNoMessage records that the field is empty unless
// something went wrong, so a host can read it without checking the status.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
func TestReport_SuccessCarriesNoMessage(t *testing.T) {
	snap := _Ran(t, BRANCHING)

	if snap.GetCause() != nil {
		t.Fatalf("want a succeeded run to have no cause, got %v", snap.GetCause())
	}

	for _, lane := range snap.GetThreads() {
		for _, node := range lane.GetLive().GetNodes() {
			if node.GetFailure() != "" {
				t.Fatalf("want %s to carry nothing, got %q", node.GetFunction(), node.GetFailure())
			}
		}
	}
}

// TestReport_TheCauseNamesItsFunctionAndThread is sub-phase 3.3, and the reason
// a pointer beats a sentence: a host follows it to a node rather than searching
// every thread for one.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation
func TestReport_TheCauseNamesItsFunctionAndThread(t *testing.T) {
	snap := _Ran(t, ONEFAILS)

	cause := snap.GetCause()
	if cause == nil {
		t.Fatal("want a cause")
	}

	if cause.GetFunction() != "bad" {
		t.Fatalf("want the function that failed, got %q", cause.GetFunction())
	}

	if cause.GetThread() == SPINE {
		t.Fatal("want the lane bad ran on, not the spine")
	}

	// Following the cause has to reach a node, which is the whole point.
	node, lane := _Node(snap, cause.GetFunction())
	if node == nil {
		t.Fatal("want the cause to name a node in the report")
	}

	if lane != cause.GetThread() {
		t.Fatalf("want the cause's thread to be the node's, got %s and %s", cause.GetThread(), lane)
	}

	if node.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want the node it names to be the failed one, got %v", node.GetStatus())
	}
}

// TestReport_AFailureOnTheSpineNamesTheSpine records the simplest case, which
// the fail-fast machinery could easily get wrong: main itself raising.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation
func TestReport_AFailureOnTheSpineNamesTheSpine(t *testing.T) {
	snap := _Ran(t, FAILING)

	cause := snap.GetCause()
	if cause == nil {
		t.Fatal("want a cause")
	}

	if cause.GetThread() != SPINE {
		t.Fatalf("want the spine, got thread %s", cause.GetThread())
	}

	if cause.GetFunction() != ENTRY {
		t.Fatalf("want %s, got %q", ENTRY, cause.GetFunction())
	}
}
