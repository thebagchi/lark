package tests

// This file feeds a Recorder report by report and checks the graph it draws
// (Drawn), the blame it keeps and the step and op types it records. A recorder
// fed one report at a time is the only way to fix the order in which threads
// report (C65a).
//
// Revisions:
//   - 2026-11-08 22:49: header states why the file is internal (C65a)
//   - 2026-11-10 00:25: moved to tests/ from recorder_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

const (
	// SPINE, LANE and OTHER are three thread ids, and BROKE the text of the
	// one failure in these tests.
	//
	// Revisions:
	//   - 2026-09-20 19:57: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPINE = "thread_0"
	LANE  = "thread_1"
	OTHER = "thread_2"
	BROKE = "it broke"
)

// _Begun is a recorder whose spine has started main, as every run's has
// before anything else is reported.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Begun() *observe.Recorder {
	into := observe.NewRecorder()
	into.Started(
		SPINE,
		&common.Line{
			Name: common.ENTRY,
		},
	)

	return into
}

// _Function is the node of the function name in the recorder's graph, or nil.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation, replacing _Thread
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Function(
	into *observe.Recorder,
	name string,
) *workflowpb.Node {
	for _, node := range into.Drawn().GetFunctions() {
		if node.GetName() == name {
			return node
		}
	}

	return nil
}

// _Calls is every edge of the recorder's graph, each as "caller callee".
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Calls(into *observe.Recorder) []string {
	var calls []string

	for _, edge := range into.Drawn().GetCalls() {
		calls = append(
			calls,
			edge.GetCaller()+" "+edge.GetCallee(),
		)
	}

	return calls
}

// TestBlame_IgnoresACancellationHoweverEarly is a property the end-to-end tests
// cannot see reliably.
//
// A thread stopped by fail-fast may report before the thread that failed - the
// two goroutines are independent, and which lands first is timing. A recorder
// that blamed whatever arrived first would then name a cancelled function as
// the cause of a failure, and only on some runs. Planting that produced a test
// that still passed, because the ordering happened to favour the failure.
//
// Asked directly, there is no ordering to be lucky about.
//
// Revisions:
//   - 2026-09-20 11:40: initial creation
//   - 2026-10-02 01:34: reports lines, and expects no index for a thread's
//     own node
//   - 2026-10-02 15:34: a cause names the function alone
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBlame_IgnoresACancellationHoweverEarly(t *testing.T) {
	var (
		into    = observe.NewRecorder()
		stopped = fmt.Errorf(
			"%s: %w",
			"patient",
			context.Canceled,
		)
	)

	into.Started(
		OTHER,
		&common.Line{
			Name: "patient",
		},
	)
	into.Ended(
		OTHER,
		"patient",
		stopped,
	)

	into.Started(
		LANE,
		&common.Line{
			Name: "bad",
		},
	)
	into.Ended(
		LANE,
		"bad",
		errors.New(BROKE),
	)

	cause := into.Cause()
	if cause.GetFunction() != "bad" || cause.GetFailure() != BROKE {
		t.Fatalf(
			"want the function that failed, and what it said, got %v",
			cause,
		)
	}
}

// TestBlame_KeepsTheLastFailure records the rule for two genuine failures:
// the last heard of, not the first, because join returns the first failure in
// argument order and settles every handle before it returns, so the function
// it returns is the last one the recorder hears fail.
//
// Revisions:
//   - 2026-09-20 11:40: initial creation
//   - 2026-10-02 15:34: starts each call before it ends, as a run does
//   - 2026-10-24 20:30: expect the last failure, not the first, matching the
//     new contract
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBlame_KeepsTheLastFailure(t *testing.T) {
	into := observe.NewRecorder()

	into.Started(
		LANE,
		&common.Line{
			Name: "first",
		},
	)
	into.Ended(
		LANE,
		"first",
		errors.New(BROKE),
	)
	into.Started(
		OTHER,
		&common.Line{
			Name: "second",
		},
	)
	into.Ended(
		OTHER,
		"second",
		errors.New("also broke"),
	)

	if into.Cause().GetFunction() != "second" {
		t.Fatalf(
			"want the last failure kept, got %q",
			into.Cause().GetFunction(),
		)
	}
}

// TestBlame_NamesTheFunctionThatFailed checks the cause is the call that
// failed, not main, which fails through it.
//
// The same error reaches both Ended calls, as it does in a real run: sign
// raises it, and main propagates it. A caller that wraps the error is still
// detected, because errors.Is follows the chain.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as TestBlame_PointsAtTheLineThatFailed
//   - 2026-10-02 15:34: names the function, there being no line to point at
//   - 2026-10-24 20:30: use the same error for both calls, as a real run does,
//     so the propagation check tells main from sign
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBlame_NamesTheFunctionThatFailed(t *testing.T) {
	into := _Begun()

	into.Started(
		SPINE,
		&common.Line{
			Name: "sign",
		},
	)
	into.Ended(
		SPINE,
		"sign",
		nil,
	)
	into.Started(
		SPINE,
		&common.Line{
			Name: "sign",
		},
	)

	broke := errors.New(BROKE)

	into.Ended(
		SPINE,
		"sign",
		broke,
	)
	into.Ended(
		SPINE,
		common.ENTRY,
		fmt.Errorf(
			"sign: %w",
			broke,
		),
	)

	if into.Cause().GetFunction() != "sign" {
		t.Fatalf(
			"want sign blamed, got %v",
			into.Cause(),
		)
	}

	if _Function(
		into,
		common.ENTRY,
	).GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf(
			"want main failed too, got %v",
			_Function(
				into,
				common.ENTRY,
			),
		)
	}
}

// TestBecause_StandsInWhenNothingWasBlamed records what a run that failed with
// no node reported says: the run's own text, no function named.
//
// A nil cause on a failed run would read to a host as "nothing went wrong",
// which is the one thing it must not say.
//
// Revisions:
//   - 2026-09-20 11:40: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBecause_StandsInWhenNothingWasBlamed(t *testing.T) {
	var (
		into  = observe.NewRecorder()
		cause = into.Because(
			workflowpb.Status_STATUS_FAILED,
			BROKE,
		)
	)

	if cause == nil {
		t.Fatal("want a failed run to carry a cause even with no node blamed")
	}

	if cause.GetFailure() != BROKE {
		t.Fatalf(
			"want the run's own text, got %q",
			cause.GetFailure(),
		)
	}

	if cause.GetFunction() != "" {
		t.Fatalf(
			"want no function named, got %q",
			cause.GetFunction(),
		)
	}

	if into.Because(
		workflowpb.Status_STATUS_CANCELLED,
		"",
	) != nil {
		t.Fatal("want no cause for a cancelled run")
	}
}

// TestStarted_TheEntryIsANodeNothingCalls checks the first call of the spine is
// main's node, running, with no edge into it.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as
//     TestStarted_MakesTheSpinesEntryItsOwnNode
//   - 2026-10-02 15:34: a node with no caller, rather than a thread's own node
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStarted_TheEntryIsANodeNothingCalls(t *testing.T) {
	var (
		into  = _Begun()
		entry = _Function(
			into,
			common.ENTRY,
		)
	)

	if entry.GetStatus() != workflowpb.Status_STATUS_RUNNING {
		t.Fatalf(
			"want main running, got %v",
			entry,
		)
	}

	if len(_Calls(into)) != 0 {
		t.Fatalf(
			"want no edge, got %v",
			_Calls(into),
		)
	}
}

// TestStarted_TwoCallsOfOneFunctionAreOneNode checks a function called twice is
// one node, and one edge from its caller, as a profiler draws it.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as
//     TestStarted_TwoCallsOfOneFunctionAreTwoLines
//   - 2026-10-02 15:34: one node and one edge, rather than two lines
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStarted_TwoCallsOfOneFunctionAreOneNode(t *testing.T) {
	into := _Begun()

	for range 2 {
		into.Started(
			SPINE,
			&common.Line{
				Name: "sign",
			},
		)
		into.Ended(
			SPINE,
			"sign",
			nil,
		)
	}

	if len(into.Drawn().GetFunctions()) != 2 {
		t.Fatalf(
			"want main and sign, got %v",
			into.Drawn().GetFunctions(),
		)
	}

	if _Function(
		into,
		"sign",
	).GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf(
			"want sign succeeded, got %v",
			_Function(
				into,
				"sign",
			),
		)
	}

	calls := _Calls(into)
	if len(calls) != 1 || calls[0] != common.ENTRY+" sign" {
		t.Fatalf(
			"want one edge from main to sign, got %v",
			calls,
		)
	}
}

// TestEnded_AnOlderCallLeavesTheNewestsStatus checks a node shows its newest
// call: an older call ending after a newer one has started changes nothing on
// the node, though a failure there is still the cause.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnded_AnOlderCallLeavesTheNewestsStatus(t *testing.T) {
	into := _Begun()

	into.Started(
		SPINE,
		&common.Line{
			Name:    "work",
			Builtin: common.SPAWN,
			Child:   LANE,
		},
	)
	into.Started(
		SPINE,
		&common.Line{
			Name: "work",
		},
	)
	into.Ended(
		SPINE,
		"work",
		nil,
	)
	into.Ended(
		LANE,
		"work",
		errors.New(BROKE),
	)

	if _Function(
		into,
		"work",
	).GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf(
			"want the newest call's status, got %v",
			_Function(
				into,
				"work",
			),
		)
	}

	if into.Cause().GetFunction() != "work" {
		t.Fatalf(
			"want the older call's failure blamed, got %v",
			into.Cause(),
		)
	}
}

// TestStarted_ARepeatIsOneCall checks a repeat is one call through all its
// attempts, closed once, while the calls the repeated function makes are its
// own edges.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as TestStarted_ARepeatAdvancesOneLine
//   - 2026-10-02 15:34: one call, rather than one line advancing
//   - 2026-10-29 00:25: counts the calls of the thread's record, which holds
//     them now
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStarted_ARepeatIsOneCall(t *testing.T) {
	into := _Begun()

	for attempt := int32(observe.ONCE); attempt <= TRIES; attempt++ {
		into.Started(
			SPINE,
			&common.Line{
				Name:    "once",
				Builtin: common.REPEAT,
				Attempt: attempt,
				Count:   TRIES,
			},
		)

		into.Started(
			SPINE,
			&common.Line{
				Name: "inner",
			},
		)
		into.Ended(
			SPINE,
			"inner",
			nil,
		)
	}

	into.Ended(
		SPINE,
		"once",
		nil,
	)

	// Only main is left: every attempt after the first was the call already
	// running, so the one end closed it.
	if len(into.Running[SPINE].Calls) != 1 {
		t.Fatalf(
			"want only main still running, got %d calls",
			len(into.Running[SPINE].Calls),
		)
	}

	if _Function(
		into,
		"once",
	).GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf(
			"want once succeeded, got %v",
			_Function(
				into,
				"once",
			),
		)
	}

	want := fmt.Sprint([]string{common.ENTRY + " once", "once inner"})
	if fmt.Sprint(_Calls(into)) != want {
		t.Fatalf(
			"got %v, want %s",
			_Calls(into),
			want,
		)
	}
}

// TestStarted_ALaterAttemptWithNothingRunningIsACall checks a later attempt
// that finds no call of its function running makes one, rather than being
// dropped.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as
//     TestStarted_ALaterAttemptOpensALineWhenNoneIsRunning
//   - 2026-10-02 15:34: a call, rather than a line
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStarted_ALaterAttemptWithNothingRunningIsACall(t *testing.T) {
	into := _Begun()

	into.Started(
		SPINE,
		&common.Line{
			Name:    "once",
			Builtin: common.RETRY,
			Attempt: TRIES,
		},
	)

	if _Function(
		into,
		"once",
	).GetStatus() != workflowpb.Status_STATUS_RUNNING {
		t.Fatalf(
			"want once running, got %v",
			_Function(
				into,
				"once",
			),
		)
	}

	if len(_Calls(into)) != 1 {
		t.Fatalf(
			"want an edge from main, got %v",
			_Calls(into),
		)
	}
}

// TestStarted_ASpawnIsACallFromTheSpawner checks a spawn is an edge from what
// the spawning thread is in, that the spawned function is the caller of what
// it calls on its own thread, and that its end, reported on that thread,
// closes it.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as
//     TestStarted_ASpawnSharesItsNodeWithTheThread
//   - 2026-10-02 15:34: an edge from the spawner, the call living on the
//     thread it started
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStarted_ASpawnIsACallFromTheSpawner(t *testing.T) {
	into := _Begun()

	into.Started(
		SPINE,
		&common.Line{
			Name:    "work",
			Builtin: common.SPAWN,
			Child:   LANE,
		},
	)
	into.Started(
		LANE,
		&common.Line{
			Name: "helper",
		},
	)
	into.Ended(
		LANE,
		"helper",
		nil,
	)
	into.Ended(
		LANE,
		"work",
		nil,
	)

	want := fmt.Sprint([]string{common.ENTRY + " work", "work helper"})
	if fmt.Sprint(_Calls(into)) != want {
		t.Fatalf(
			"got %v, want %s",
			_Calls(into),
			want,
		)
	}

	if _Function(
		into,
		"work",
	).GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf(
			"want work succeeded, got %v",
			_Function(
				into,
				"work",
			),
		)
	}

	_, kept := into.Running[LANE]
	if kept {
		t.Fatalf(
			"want the finished thread forgotten, got %v",
			into.Running[LANE],
		)
	}
}

// TestEnded_ALineThatCallsNothingIsNoNode checks a join, a sleep and a cancel,
// which report no function, leave the graph as it was.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as
//     TestEnded_AnEmptyNameClosesWhatCalledNothing
//   - 2026-10-02 15:34: no node at all
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnded_ALineThatCallsNothingIsNoNode(t *testing.T) {
	into := _Begun()

	for _, builtin := range []string{common.JOIN, common.SLEEP, common.CANCEL} {
		into.Started(
			SPINE,
			&common.Line{
				Builtin: builtin,
			},
		)
		into.Ended(
			SPINE,
			"",
			errors.New(BROKE),
		)
	}

	if len(into.Drawn().GetFunctions()) != 1 || len(_Calls(into)) != 0 {
		t.Fatalf(
			"want main alone, got %v",
			into.Drawn(),
		)
	}

	if into.Cause() != nil {
		t.Fatalf(
			"want nothing blamed, got %v",
			into.Cause(),
		)
	}
}

// TestEnded_AnEndWithNoCallChangesNothing checks an end the recorder was never
// told began makes no node and blames nothing.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as TestEnded_AMissClosesTheThread
//   - 2026-10-02 15:34: changes nothing, there being no thread to close
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnded_AnEndWithNoCallChangesNothing(t *testing.T) {
	into := _Begun()

	into.Ended(
		LANE,
		"ghost",
		errors.New(BROKE),
	)

	if _Function(
		into,
		"ghost",
	) != nil || into.Cause() != nil {
		t.Fatalf(
			"want nothing recorded, got %v",
			into.Drawn(),
		)
	}
}

// TestEnded_ADeadlineCancelsWithoutACause checks a deadline is a cancellation.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation
//   - 2026-10-02 15:34: reads the function's node
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnded_ADeadlineCancelsWithoutACause(t *testing.T) {
	into := _Begun()

	into.Started(
		SPINE,
		&common.Line{
			Name: "slow",
		},
	)
	into.Ended(
		SPINE,
		"slow",
		context.DeadlineExceeded,
	)

	if _Function(
		into,
		"slow",
	).GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf(
			"want a cancellation, got %v",
			_Function(
				into,
				"slow",
			),
		)
	}

	if into.Cause() != nil {
		t.Fatalf(
			"want no cause, got %v",
			into.Cause(),
		)
	}
}

// TestDrawn_IsSortedByName checks functions come by name and calls by caller
// and then callee, whatever order the run made them in.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as TestThreads_AreInTreeOrder
//   - 2026-10-02 15:34: sorted by name, there being no tree
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestDrawn_IsSortedByName(t *testing.T) {
	into := _Begun()

	for _, name := range []string{"zeta", "alpha", "mu"} {
		into.Started(
			SPINE,
			&common.Line{
				Name: name,
			},
		)
		into.Ended(
			SPINE,
			name,
			nil,
		)
	}

	var names []string

	for _, node := range into.Drawn().GetFunctions() {
		names = append(
			names,
			node.GetName(),
		)
	}

	want := fmt.Sprint([]string{"alpha", common.ENTRY, "mu", "zeta"})
	if fmt.Sprint(names) != want {
		t.Fatalf(
			"got %v, want %s",
			names,
			want,
		)
	}

	calls := fmt.Sprint([]string{"main alpha", "main mu", "main zeta"})
	if fmt.Sprint(_Calls(into)) != calls {
		t.Fatalf(
			"got %v, want %s",
			_Calls(into),
			calls,
		)
	}
}

// TestDrawn_StaysAsItWas checks a handed-out graph does not follow a later
// write. The graph is what a host is holding; the recorder keeps writing to
// its own nodes after that.
//
// Revisions:
//   - 2026-10-02 08:37: initial creation, as TestThreads_ASnapshotStaysAsItWas
//   - 2026-10-02 15:34: reads a function's node
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestDrawn_StaysAsItWas(t *testing.T) {
	into := _Begun()

	into.Started(
		SPINE,
		&common.Line{
			Name: "sign",
		},
	)

	shot := into.Drawn()

	into.Ended(
		SPINE,
		"sign",
		errors.New(BROKE),
	)

	for _, node := range shot.GetFunctions() {
		if node.GetStatus() != workflowpb.Status_STATUS_RUNNING {
			t.Fatalf(
				"want the graph left running, got %v",
				node,
			)
		}
	}
}

// _Told is every operation a recorder told its watcher, one line each: the op,
// the path, and the value as compact JSON when there is one.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Told(
	t *testing.T,
	into *observe.Recorder,
) *[]string {
	t.Helper()

	told := new([]string)

	into.Watch = func(change *workflowpb.Change) {
		for _, op := range change.GetOperations() {
			line := op.GetOp() + " " + op.GetPath()

			if op.GetValue() != nil {
				raw, err := json.Marshal(op.GetValue().AsInterface())
				if err != nil {
					t.Fatalf(
						"value of %s: %v",
						op.GetPath(),
						err,
					)
				}

				line += " " + string(raw)
			}

			*told = append(
				*told,
				line,
			)
		}
	}

	return told
}

// TestMembers_AreTheSchemasJSONNames checks the members a change's paths name
// are what protojson writes for the schema's fields, so renaming a field
// cannot leave the changes pointing at a member that is not there.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestMembers_AreTheSchemasJSONNames(t *testing.T) {
	var (
		graph = (&workflowpb.Graph{}).ProtoReflect().Descriptor().Fields()
		node  = (&workflowpb.Node{}).ProtoReflect().Descriptor().Fields()
	)

	for member, field := range map[string]protoreflect.FieldDescriptor{
		observe.FUNCTIONS: graph.ByName(observe.FUNCTIONS),
		observe.CALLS:     graph.ByName(observe.CALLS),
		observe.STATUS:    graph.ByName(observe.STATUS),
		observe.CAUSE:     graph.ByName(observe.CAUSE),
		observe.THREADS:   node.ByName(observe.THREADS),
	} {
		if field == nil || field.JSONName() != member {
			t.Fatalf(
				"no field written as %q",
				member,
			)
		}
	}
}

// TestChanges_SayWhatEachStepDid checks the operations a watcher is told for
// main calling fetch: each node added in its place, each thread moved to the
// function it executes, the edge, and each status, with an empty list a member
// that is not there, as protojson writes it.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestChanges_SayWhatEachStepDid(t *testing.T) {
	var (
		into = observe.NewRecorder()
		told = _Told(
			t,
			into,
		)
	)

	into.Started(
		SPINE,
		&common.Line{
			Name: common.ENTRY,
		},
	)
	into.Started(
		SPINE,
		&common.Line{
			Name: "fetch",
		},
	)
	into.Ended(
		SPINE,
		"fetch",
		nil,
	)
	into.Ended(
		SPINE,
		common.ENTRY,
		nil,
	)

	want := []string{
		`add /functions [{"name":"main","status":"STATUS_RUNNING"}]`,
		`add /functions/0/threads ["thread_0"]`,
		`add /functions/0 {"name":"fetch","status":"STATUS_RUNNING"}`,
		`remove /functions/1/threads`,
		`add /functions/0/threads ["thread_0"]`,
		`add /calls [{"callee":"fetch","caller":"main"}]`,
		`remove /functions/0/threads`,
		`add /functions/1/threads ["thread_0"]`,
		`add /functions/0/status "STATUS_SUCCEEDED"`,
		`remove /functions/1/threads`,
		`add /functions/1/status "STATUS_SUCCEEDED"`,
	}

	if strings.Join(
		*told,
		"\n",
	) != strings.Join(
		want,
		"\n",
	) {
		t.Fatalf(
			"told\n%s\nwant\n%s",
			strings.Join(
				*told,
				"\n",
			),
			strings.Join(
				want,
				"\n",
			),
		)
	}
}

// TestChanges_ShowWhichThreadExecutesWhat checks two threads executing one
// function are both on its node, in the order they arrived, and that one
// leaving takes only itself off.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestChanges_ShowWhichThreadExecutesWhat(t *testing.T) {
	into := _Begun()

	into.Started(
		SPINE,
		&common.Line{
			Name:    "work",
			Builtin: common.SPAWN,
			Child:   LANE,
		},
	)
	into.Started(
		SPINE,
		&common.Line{
			Name:    "work",
			Builtin: common.SPAWN,
			Child:   OTHER,
		},
	)

	threads := fmt.Sprint(_Function(
		into,
		"work",
	).GetThreads())
	if threads != fmt.Sprint([]string{LANE, OTHER}) {
		t.Fatalf(
			"want both threads on work, got %s",
			threads,
		)
	}

	into.Ended(
		LANE,
		"work",
		nil,
	)

	threads = fmt.Sprint(_Function(
		into,
		"work",
	).GetThreads())
	if threads != fmt.Sprint([]string{OTHER}) {
		t.Fatalf(
			"want only %s left on work, got %s",
			OTHER,
			threads,
		)
	}

	spine := fmt.Sprint(_Function(
		into,
		common.ENTRY,
	).GetThreads())
	if spine != fmt.Sprint([]string{SPINE}) {
		t.Fatalf(
			"want the spine still on main, got %s",
			spine,
		)
	}
}

// _Stream is every operation a recorder tells its watcher for a run of four
// threads, one line each, and the lines joined by newlines.
//
// The run adds nodes and edges first, last and between others, puts three
// threads on one function, takes one off the middle, the first and the last,
// and puts a thread on the emptied function again. Every step is in one
// goroutine, so the stream is the same on every run.
//
// Revisions:
//   - 2026-11-02 00:54: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Stream(t *testing.T) string {
	t.Helper()

	var (
		into = observe.NewRecorder()
		told = _Told(
			t,
			into,
		)
		threads = []string{
			LANE,
			OTHER,
			"thread_3",
			"thread_4",
		}
	)

	into.Started(
		SPINE,
		&common.Line{
			Name: common.ENTRY,
		},
	)

	for _, name := range []string{"mid", "zed", "alpha"} {
		into.Started(
			SPINE,
			&common.Line{
				Name: name,
			},
		)
		into.Ended(
			SPINE,
			name,
			nil,
		)
	}

	for _, thread := range threads[:3] {
		into.Started(
			SPINE,
			&common.Line{
				Name:    "work",
				Builtin: common.SPAWN,
				Child:   thread,
			},
		)
	}

	for _, thread := range []string{
		OTHER,
		LANE,
		"thread_3",
	} {
		into.Ended(
			thread,
			"work",
			nil,
		)
	}

	into.Started(
		SPINE,
		&common.Line{
			Name:    "work",
			Builtin: common.SPAWN,
			Child:   threads[3],
		},
	)
	into.Ended(
		threads[3],
		"work",
		nil,
	)
	into.Ended(
		SPINE,
		common.ENTRY,
		nil,
	)

	return strings.Join(
		*told,
		"\n",
	) + "\n"
}

const (
	// STREAM is the stream of operations a recorder tells its watcher for a run of
	// four threads, one operation to a line.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/stream.golden (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	STREAM = `
add /functions [{"name":"main","status":"STATUS_RUNNING"}]
add /functions/0/threads ["thread_0"]
add /functions/1 {"name":"mid","status":"STATUS_RUNNING"}
remove /functions/0/threads
add /functions/1/threads ["thread_0"]
add /calls [{"callee":"mid","caller":"main"}]
remove /functions/1/threads
add /functions/0/threads ["thread_0"]
add /functions/1/status "STATUS_SUCCEEDED"
add /functions/2 {"name":"zed","status":"STATUS_RUNNING"}
remove /functions/0/threads
add /functions/2/threads ["thread_0"]
add /calls/1 {"callee":"zed","caller":"main"}
remove /functions/2/threads
add /functions/0/threads ["thread_0"]
add /functions/2/status "STATUS_SUCCEEDED"
add /functions/0 {"name":"alpha","status":"STATUS_RUNNING"}
remove /functions/1/threads
add /functions/0/threads ["thread_0"]
add /calls/0 {"callee":"alpha","caller":"main"}
remove /functions/0/threads
add /functions/1/threads ["thread_0"]
add /functions/0/status "STATUS_SUCCEEDED"
add /functions/3 {"name":"work","status":"STATUS_RUNNING"}
add /functions/3/threads ["thread_1"]
add /calls/2 {"callee":"work","caller":"main"}
add /functions/3/status "STATUS_RUNNING"
add /functions/3/threads/- "thread_2"
add /functions/3/status "STATUS_RUNNING"
add /functions/3/threads/- "thread_3"
remove /functions/3/threads/1
remove /functions/3/threads/0
remove /functions/3/threads
add /functions/3/status "STATUS_SUCCEEDED"
add /functions/3/status "STATUS_RUNNING"
add /functions/3/threads ["thread_4"]
remove /functions/3/threads
add /functions/3/status "STATUS_SUCCEEDED"
remove /functions/1/threads
add /functions/1/status "STATUS_SUCCEEDED"
`
)

// TestChanges_StreamOfFourThreadsIsAsRecorded checks the full stream of
// operations against STREAM, byte for byte. The golden text was
// captured before the add and remove helpers replaced the four sites that
// build a list operation.
//
// Revisions:
//   - 2026-11-02 00:54: initial creation
//   - 2026-11-02 00:54: corrects the count of sites to four
//   - 2026-11-09 11:25: reads STREAM, not testdata/stream.golden (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestChanges_StreamOfFourThreadsIsAsRecorded(t *testing.T) {
	got := _Stream(t)
	if got != strings.TrimPrefix(
		STREAM,
		"\n",
	) {
		t.Fatalf(
			"stream\n%s\nwant\n%s",
			got,
			strings.TrimPrefix(
				STREAM,
				"\n",
			),
		)
	}
}

// TestOp_OneAllocationHoldsAStepsOperations checks that a step makes room for
// its most operations once. Every change a watcher is told must have capacity
// STEP_CAPACITY and at most STEP_CAPACITY operations, and four operations must cost one
// slice plus four operations, not the three slices of a slice that grows 1,
// 2, 4.
//
// Success: the test passes. A regression shows as a capacity other than
// STEP_CAPACITY, a length above it, or more than five allocations.
//
// Revisions:
//   - 2026-11-02 16:52: initial creation
//   - 2026-11-02 17:01: uses STEP_CAPACITY, renamed from STEPOPS
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOp_OneAllocationHoldsAStepsOperations(t *testing.T) {
	t.Run(
		"every told change fits STEP_CAPACITY",
		func(t *testing.T) {
			var (
				into = observe.NewRecorder()
				most = 0
			)

			into.Watch = func(change *workflowpb.Change) {
				ops := change.GetOperations()
				if cap(ops) != observe.STEP_CAPACITY {
					t.Errorf(
						"cap = %d, want %d",
						cap(ops),
						observe.STEP_CAPACITY,
					)
				}

				most = max(
					most,
					len(ops),
				)
			}

			into.Started(
				SPINE,
				&common.Line{
					Name: common.ENTRY,
				},
			)

			for _, name := range []string{"mid", "zed", "alpha"} {
				into.Started(
					SPINE,
					&common.Line{
						Name: name,
					},
				)
				into.Ended(
					SPINE,
					name,
					nil,
				)
			}

			if most != observe.STEP_CAPACITY {
				t.Errorf(
					"most operations in a step = %d, want %d",
					most,
					observe.STEP_CAPACITY,
				)
			}
		},
	)

	t.Run(
		"four operations cost five allocations",
		func(t *testing.T) {
			var (
				into = observe.NewRecorder()
				got  float64
			)

			got = testing.AllocsPerRun(
				100,
				func() {
					for range observe.STEP_CAPACITY {
						into.Op(
							observe.ADD,
							"",
							nil,
						)
					}

					into.Ops = nil
				},
			)

			if got > observe.STEP_CAPACITY+1 {
				t.Errorf(
					"allocs = %v, want at most %d",
					got,
					observe.STEP_CAPACITY+1,
				)
			}
		},
	)
}
