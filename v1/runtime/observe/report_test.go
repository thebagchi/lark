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
	RETRYSLOW   = "testdata/retryslow.star"
	SLOWSIBLING = "testdata/slowsibling.star"
	RAISING     = "testdata/raising.star"

	// BREAKS is the text the failing sample asserts with, and RAISES the text
	// the raising one fails with.
	BREAKS = "this one breaks"
	RAISES = "this one raises"

	// UNREACHED is a function the branching script declares and never calls.
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
//   - 2026-10-02 01:35: returns a Graph
//   - 2026-10-02 13:12: takes no options, Start having none
func _Ran(t *testing.T, path string) *workflowpb.Graph {
	t.Helper()

	run := observe.Start(t.Context(), _Compile(t, path))

	_, err := run.Wait()
	if err != nil {
		// How the run ended is what the snapshot below reports, and several
		// of these scripts end badly on purpose.
		t.Logf("%s ended with: %v", path, err)
	}

	return run.Status()
}

// _Node is the node of the function name in the graph, or nil when the run
// never called it.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
//   - 2026-10-02 01:35: reads a thread's own node before its lines
//   - 2026-10-02 15:34: one node per function, so there is no thread to say
func _Node(snap *workflowpb.Graph, name string) *workflowpb.Node {
	for _, node := range snap.GetFunctions() {
		if node.GetName() == name {
			return node
		}
	}

	return nil
}

// _Called is whether the graph has an edge from caller to callee.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
func _Called(snap *workflowpb.Graph, caller string, callee string) bool {
	for _, edge := range snap.GetCalls() {
		if edge.GetCaller() == caller && edge.GetCallee() == callee {
			return true
		}
	}

	return false
}

// TestReport_NamesEveryFunctionThatRan is §7.2: every function the run called
// is a node, and a spawn is an edge from the function that spawned it.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
//   - 2026-10-02 15:34: an edge from main to each spawned function, there
//     being no threads
func TestReport_NamesEveryFunctionThatRan(t *testing.T) {
	snap := _Ran(t, BRANCHING)

	if _Node(snap, ENTRY) == nil {
		t.Fatal("want the entry point reported")
	}

	for _, name := range []string{"alpha", "beta"} {
		node := _Node(snap, name)
		if node.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
			t.Fatalf("want %s succeeded, got %v", name, node)
		}

		if !_Called(snap, ENTRY, name) {
			t.Fatalf("want an edge from main to %s, got %v", name, snap.GetCalls())
		}
	}
}

// TestReport_LeavesOutWhatNothingCalled is the default chosen on 2026-09-20
// 01:05: without a graph, a report says what the run did.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestReport_LeavesOutWhatNothingCalled(t *testing.T) {
	node := _Node(_Ran(t, BRANCHING), UNREACHED)

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

	bad := _Node(snap, "bad")
	if bad.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want bad failed, got %v", bad)
	}

	good := _Node(snap, "good")
	if good == nil || good.GetStatus() == workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want good not failed, got %v", good)
	}
}

// TestReport_ARetryIsOneSucceededNode is §7.3 through a retry: three attempts
// of one function are one node, which reads as the retry ended.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation, as
//     TestReport_CarriesTheAttemptARetryReached
//   - 2026-10-02 15:34: one node that succeeded, a node carrying no attempt
func TestReport_ARetryIsOneSucceededNode(t *testing.T) {
	snap := _Ran(t, RETRYING)

	node := _Node(snap, "flaky")
	if node.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want the retried function succeeded in the end, got %v", node)
	}

	if !_Called(snap, ENTRY, "flaky") {
		t.Fatalf("want an edge from main to flaky, got %v", snap.GetCalls())
	}
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
//   - 2026-10-02 15:34: polls the function's node, which carries no attempt
func TestReport_NeverShowsACaughtFailure(t *testing.T) {
	run := observe.Start(t.Context(), _Compile(t, RETRYSLOW))

	seen := 0

	for {
		snap := run.Status()

		node := _Node(snap, "flaky")
		if node != nil {
			seen++

			if node.GetStatus() == workflowpb.Status_STATUS_FAILED {
				t.Fatal("a caught attempt was reported failed while retrying")
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
}

// TestReport_CountsEveryRepeat records that three calls of a repeat are one
// node, succeeded once the repeat is.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
//   - 2026-10-02 15:34: one node, which carries no attempt
func TestReport_CountsEveryRepeat(t *testing.T) {
	snap := _Ran(t, REPEATING)

	node := _Node(snap, "step")
	if node.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want the repeated function succeeded, got %v", node)
	}

	if len(snap.GetFunctions()) != 2 {
		t.Fatalf("want main and step alone, got %v", snap.GetFunctions())
	}
}

// TestReport_CancelledIsNotFailed is sub-phase 3.1: a run a caller stopped did
// not break, and the schema can finally say so.
//
// Revisions:
//   - 2026-09-20 11:34: initial creation
//   - 2026-10-03 00:22: stops the run by cancelling its context, the one way to stop one
func TestReport_CancelledIsNotFailed(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	run := observe.Start(ctx, _Compile(t, SLOW))

	stop()

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

	bad := _Node(snap, "bad")
	if bad.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want the function that asserted to be failed, got %v", bad)
	}

	slow := _Node(snap, "patient")
	if slow.GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf("want the sibling cancelled rather than failed, got %v", slow)
	}
}

// TestReport_AFailureCarriesItsMessage is sub-phase 3.2: a status says a run
// failed, and the cause says what went wrong.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
//   - 2026-10-02 15:34: the message is the cause's, a node carrying none
func TestReport_AFailureCarriesItsMessage(t *testing.T) {
	snap := _Ran(t, ONEFAILS)

	if !strings.Contains(snap.GetCause().GetFailure(), BREAKS) {
		t.Fatalf("want the assertion's own words, got %v", snap.GetCause())
	}
}

// TestReport_ACancellationCarriesNoMessage is the other half of the rule: a
// cancelled run has no cause, since nothing went wrong in it.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
//   - 2026-10-02 15:34: reads the cause alone, a node carrying no message
//   - 2026-10-03 00:22: stops the run by cancelling its context, the one way to stop one
func TestReport_ACancellationCarriesNoMessage(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	run := observe.Start(ctx, _Compile(t, SLOW))

	stop()

	_, err := run.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want the cancellation, got %v", err)
	}

	if run.Status().GetCause() != nil {
		t.Fatalf("want a cancelled run to have no cause, got %v", run.Status().GetCause())
	}
}

// TestReport_SuccessCarriesNoMessage records that a run that succeeded has no
// cause, so a host can read the pointer without checking the status.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
//   - 2026-10-02 01:35: reads every node of a Graph, lines included
//   - 2026-10-02 15:34: reads the cause alone, a node carrying no message
func TestReport_SuccessCarriesNoMessage(t *testing.T) {
	snap := _Ran(t, BRANCHING)

	if snap.GetCause() != nil {
		t.Fatalf("want a succeeded run to have no cause, got %v", snap.GetCause())
	}
}

// TestReport_TheCauseNamesItsFunction is sub-phase 3.3, and the reason a
// pointer beats a sentence: a host follows it to a node rather than searching
// for one.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation, as
//     TestReport_TheCauseNamesItsFunctionAndThread
//   - 2026-10-02 15:34: names the function, a graph having no threads
func TestReport_TheCauseNamesItsFunction(t *testing.T) {
	snap := _Ran(t, ONEFAILS)

	cause := snap.GetCause()
	if cause.GetFunction() != "bad" {
		t.Fatalf("want the function that failed, got %v", cause)
	}

	// Following the cause has to reach a node, which is the whole point.
	node := _Node(snap, cause.GetFunction())
	if node.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want the node it names to be the failed one, got %v", node)
	}
}

// TestReport_AFailureOfTheEntryNamesTheEntry records the simplest case, which
// the fail-fast machinery could easily get wrong: main itself raising.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation, as
//     TestReport_AFailureOnTheSpineNamesTheSpine
//   - 2026-10-02 15:34: names main, a graph having no spine
func TestReport_AFailureOfTheEntryNamesTheEntry(t *testing.T) {
	snap := _Ran(t, FAILING)

	if snap.GetCause().GetFunction() != ENTRY {
		t.Fatalf("want %s, got %v", ENTRY, snap.GetCause())
	}
}

// TestReport_AnEntryPointThatRaisesFails records main raising with nothing
// recording it as the run's outcome - Starlark's own fail rather than assert -
// which once left main succeeded beside a failed run, and the cause pointing
// nowhere.
//
// Revisions:
//   - 2026-10-02 12:19: initial creation, as
//     TestReport_AnEntryPointThatRaisesFailsTheSpine
//   - 2026-10-02 15:34: reads main's node, and the cause for what it raised
func TestReport_AnEntryPointThatRaisesFails(t *testing.T) {
	snap := _Ran(t, RAISING)

	if _Node(snap, ENTRY).GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want main failed, got %v", _Node(snap, ENTRY))
	}

	cause := snap.GetCause()

	blamed := cause.GetFunction() == ENTRY && strings.Contains(cause.GetFailure(), RAISES)
	if !blamed {
		t.Fatalf("want main blamed, with what it raised, got %v", cause)
	}
}

// TestReport_AStoppedRunCancelsTheEntry records main's node ending as the run
// did when a host stopped it: cancelled, rather than succeeded beside a
// cancelled run.
//
// Revisions:
//   - 2026-10-02 12:19: initial creation, as TestReport_AStoppedRunCancelsTheSpine
//   - 2026-10-02 15:34: reads main's node
//   - 2026-10-03 00:22: stops the run by cancelling its context, the one way to stop one
func TestReport_AStoppedRunCancelsTheEntry(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	run := observe.Start(ctx, _Compile(t, SLOW))

	stop()

	_, err := run.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want the cancellation, got %v", err)
	}

	entry := _Node(run.Status(), ENTRY)
	if entry.GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf("want main cancelled, got %v", entry)
	}
}
