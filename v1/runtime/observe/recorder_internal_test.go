package observe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// SPINE, LANE and OTHER are three thread ids, and BROKE the text of the
	// one failure in these tests.
	SPINE = "thread_0"
	LANE  = "thread_1"
	OTHER = "thread_2"
	BROKE = "it broke"

	// TRIES is how many attempts the repeat below makes.
	TRIES = 3
)

// _Begun is a recorder whose spine has started main, as every run's has
// before anything else is reported.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation
func _Begun() *_Recorder {
	into := _NewRecorder()
	into.Started(SPINE, &scheduler.Line{Name: spelling.ENTRY})

	return into
}

// _Function is the node of the function name in the recorder's graph, or nil.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation, replacing _Thread
func _Function(into *_Recorder, name string) *workflowpb.Node {
	for _, node := range into._Drawn().GetFunctions() {
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
func _Calls(into *_Recorder) []string {
	var calls []string

	for _, edge := range into._Drawn().GetCalls() {
		calls = append(calls, edge.GetCaller()+" "+edge.GetCallee())
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
func TestBlame_IgnoresACancellationHoweverEarly(t *testing.T) {
	into := _NewRecorder()

	stopped := fmt.Errorf("%s: %w", "patient", context.Canceled)

	into.Started(OTHER, &scheduler.Line{Name: "patient"})
	into.Ended(OTHER, "patient", stopped)

	into.Started(LANE, &scheduler.Line{Name: "bad"})
	into.Ended(LANE, "bad", errors.New(BROKE))

	cause := into._Cause()
	if cause.GetFunction() != "bad" || cause.GetFailure() != BROKE {
		t.Fatalf("want the function that failed, and what it said, got %v", cause)
	}
}

// TestBlame_KeepsTheFirstFailure records the rule for two genuine failures:
// the first heard of, not the last.
//
// Revisions:
//   - 2026-09-20 11:40: initial creation
//   - 2026-10-02 15:34: starts each call before it ends, as a run does
func TestBlame_KeepsTheFirstFailure(t *testing.T) {
	into := _NewRecorder()

	into.Started(LANE, &scheduler.Line{Name: "first"})
	into.Ended(LANE, "first", errors.New(BROKE))
	into.Started(OTHER, &scheduler.Line{Name: "second"})
	into.Ended(OTHER, "second", errors.New("also broke"))

	if into._Cause().GetFunction() != "first" {
		t.Fatalf("want the first failure kept, got %q", into._Cause().GetFunction())
	}
}

// TestBlame_NamesTheFunctionThatFailed checks the cause is the call that
// failed, not main, which fails through it.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as TestBlame_PointsAtTheLineThatFailed
//   - 2026-10-02 15:34: names the function, there being no line to point at
func TestBlame_NamesTheFunctionThatFailed(t *testing.T) {
	into := _Begun()

	into.Started(SPINE, &scheduler.Line{Name: "sign"})
	into.Ended(SPINE, "sign", nil)
	into.Started(SPINE, &scheduler.Line{Name: "sign"})
	into.Ended(SPINE, "sign", errors.New(BROKE))
	into.Ended(SPINE, spelling.ENTRY, errors.New(BROKE))

	if into._Cause().GetFunction() != "sign" {
		t.Fatalf("want sign blamed, got %v", into._Cause())
	}

	if _Function(into, spelling.ENTRY).GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want main failed too, got %v", _Function(into, spelling.ENTRY))
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
func TestBecause_StandsInWhenNothingWasBlamed(t *testing.T) {
	into := _NewRecorder()

	cause := into._Because(workflowpb.Status_STATUS_FAILED, BROKE)
	if cause == nil {
		t.Fatal("want a failed run to carry a cause even with no node blamed")
	}

	if cause.GetFailure() != BROKE {
		t.Fatalf("want the run's own text, got %q", cause.GetFailure())
	}

	if cause.GetFunction() != "" {
		t.Fatalf("want no function named, got %q", cause.GetFunction())
	}

	if into._Because(workflowpb.Status_STATUS_CANCELLED, "") != nil {
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
func TestStarted_TheEntryIsANodeNothingCalls(t *testing.T) {
	into := _Begun()

	entry := _Function(into, spelling.ENTRY)
	if entry.GetStatus() != workflowpb.Status_STATUS_RUNNING {
		t.Fatalf("want main running, got %v", entry)
	}

	if len(_Calls(into)) != 0 {
		t.Fatalf("want no edge, got %v", _Calls(into))
	}
}

// TestStarted_TwoCallsOfOneFunctionAreOneNode checks a function called twice is
// one node, and one edge from its caller, as a profiler draws it.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as
//     TestStarted_TwoCallsOfOneFunctionAreTwoLines
//   - 2026-10-02 15:34: one node and one edge, rather than two lines
func TestStarted_TwoCallsOfOneFunctionAreOneNode(t *testing.T) {
	into := _Begun()

	for range 2 {
		into.Started(SPINE, &scheduler.Line{Name: "sign"})
		into.Ended(SPINE, "sign", nil)
	}

	if len(into._Drawn().GetFunctions()) != 2 {
		t.Fatalf("want main and sign, got %v", into._Drawn().GetFunctions())
	}

	if _Function(into, "sign").GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want sign succeeded, got %v", _Function(into, "sign"))
	}

	calls := _Calls(into)
	if len(calls) != 1 || calls[0] != spelling.ENTRY+" sign" {
		t.Fatalf("want one edge from main to sign, got %v", calls)
	}
}

// TestEnded_AnOlderCallLeavesTheNewestsStatus checks a node shows its newest
// call: an older call ending after a newer one has started changes nothing on
// the node, though a failure there is still the cause.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
func TestEnded_AnOlderCallLeavesTheNewestsStatus(t *testing.T) {
	into := _Begun()

	into.Started(SPINE, &scheduler.Line{Name: "work", Builtin: spelling.SPAWN, Child: LANE})
	into.Started(SPINE, &scheduler.Line{Name: "work"})
	into.Ended(SPINE, "work", nil)
	into.Ended(LANE, "work", errors.New(BROKE))

	if _Function(into, "work").GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want the newest call's status, got %v", _Function(into, "work"))
	}

	if into._Cause().GetFunction() != "work" {
		t.Fatalf("want the older call's failure blamed, got %v", into._Cause())
	}
}

// TestStarted_ARepeatIsOneCall checks a repeat is one call through all its
// attempts, closed once, while the calls the repeated function makes are its
// own edges.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as TestStarted_ARepeatAdvancesOneLine
//   - 2026-10-02 15:34: one call, rather than one line advancing
func TestStarted_ARepeatIsOneCall(t *testing.T) {
	into := _Begun()

	for attempt := int32(ONCE); attempt <= TRIES; attempt++ {
		into.Started(SPINE, &scheduler.Line{
			Name:    "once",
			Builtin: spelling.REPEAT,
			Attempt: attempt,
			Count:   TRIES,
		})

		into.Started(SPINE, &scheduler.Line{Name: "inner"})
		into.Ended(SPINE, "inner", nil)
	}

	into.Ended(SPINE, "once", nil)

	// Only main is left: every attempt after the first was the call already
	// running, so the one end closed it.
	if len(into.running[SPINE]) != 1 {
		t.Fatalf("want only main still running, got %d calls", len(into.running[SPINE]))
	}

	if _Function(into, "once").GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want once succeeded, got %v", _Function(into, "once"))
	}

	want := fmt.Sprint([]string{spelling.ENTRY + " once", "once inner"})
	if fmt.Sprint(_Calls(into)) != want {
		t.Fatalf("got %v, want %s", _Calls(into), want)
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
func TestStarted_ALaterAttemptWithNothingRunningIsACall(t *testing.T) {
	into := _Begun()

	into.Started(SPINE, &scheduler.Line{Name: "once", Builtin: spelling.RETRY, Attempt: TRIES})

	if _Function(into, "once").GetStatus() != workflowpb.Status_STATUS_RUNNING {
		t.Fatalf("want once running, got %v", _Function(into, "once"))
	}

	if len(_Calls(into)) != 1 {
		t.Fatalf("want an edge from main, got %v", _Calls(into))
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
func TestStarted_ASpawnIsACallFromTheSpawner(t *testing.T) {
	into := _Begun()

	into.Started(SPINE, &scheduler.Line{Name: "work", Builtin: spelling.SPAWN, Child: LANE})
	into.Started(LANE, &scheduler.Line{Name: "helper"})
	into.Ended(LANE, "helper", nil)
	into.Ended(LANE, "work", nil)

	want := fmt.Sprint([]string{spelling.ENTRY + " work", "work helper"})
	if fmt.Sprint(_Calls(into)) != want {
		t.Fatalf("got %v, want %s", _Calls(into), want)
	}

	if _Function(into, "work").GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want work succeeded, got %v", _Function(into, "work"))
	}

	_, kept := into.running[LANE]
	if kept {
		t.Fatalf("want the finished thread forgotten, got %v", into.running[LANE])
	}
}

// TestEnded_ALineThatCallsNothingIsNoNode checks a join, a sleep and a cancel,
// which report no function, leave the graph as it was.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as
//     TestEnded_AnEmptyNameClosesWhatCalledNothing
//   - 2026-10-02 15:34: no node at all
func TestEnded_ALineThatCallsNothingIsNoNode(t *testing.T) {
	into := _Begun()

	for _, builtin := range []string{spelling.JOIN, spelling.SLEEP, spelling.CANCEL} {
		into.Started(SPINE, &scheduler.Line{Builtin: builtin})
		into.Ended(SPINE, "", errors.New(BROKE))
	}

	if len(into._Drawn().GetFunctions()) != 1 || len(_Calls(into)) != 0 {
		t.Fatalf("want main alone, got %v", into._Drawn())
	}

	if into._Cause() != nil {
		t.Fatalf("want nothing blamed, got %v", into._Cause())
	}
}

// TestEnded_AnEndWithNoCallChangesNothing checks an end the recorder was never
// told began makes no node and blames nothing.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as TestEnded_AMissClosesTheThread
//   - 2026-10-02 15:34: changes nothing, there being no thread to close
func TestEnded_AnEndWithNoCallChangesNothing(t *testing.T) {
	into := _Begun()

	into.Ended(LANE, "ghost", errors.New(BROKE))

	if _Function(into, "ghost") != nil || into._Cause() != nil {
		t.Fatalf("want nothing recorded, got %v", into._Drawn())
	}
}

// TestEnded_ADeadlineCancelsWithoutACause checks a deadline is a cancellation.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation
//   - 2026-10-02 15:34: reads the function's node
func TestEnded_ADeadlineCancelsWithoutACause(t *testing.T) {
	into := _Begun()

	into.Started(SPINE, &scheduler.Line{Name: "slow"})
	into.Ended(SPINE, "slow", context.DeadlineExceeded)

	if _Function(into, "slow").GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf("want a cancellation, got %v", _Function(into, "slow"))
	}

	if into._Cause() != nil {
		t.Fatalf("want no cause, got %v", into._Cause())
	}
}

// TestDrawn_IsSortedByName checks functions come by name and calls by caller
// and then callee, whatever order the run made them in.
//
// Revisions:
//   - 2026-10-02 01:34: initial creation, as TestThreads_AreInTreeOrder
//   - 2026-10-02 15:34: sorted by name, there being no tree
func TestDrawn_IsSortedByName(t *testing.T) {
	into := _Begun()

	for _, name := range []string{"zeta", "alpha", "mu"} {
		into.Started(SPINE, &scheduler.Line{Name: name})
		into.Ended(SPINE, name, nil)
	}

	var names []string

	for _, node := range into._Drawn().GetFunctions() {
		names = append(names, node.GetName())
	}

	want := fmt.Sprint([]string{"alpha", spelling.ENTRY, "mu", "zeta"})
	if fmt.Sprint(names) != want {
		t.Fatalf("got %v, want %s", names, want)
	}

	calls := fmt.Sprint([]string{"main alpha", "main mu", "main zeta"})
	if fmt.Sprint(_Calls(into)) != calls {
		t.Fatalf("got %v, want %s", _Calls(into), calls)
	}
}

// TestDrawn_StaysAsItWas checks a handed-out graph does not follow a later
// write. The graph is what a host is holding; the recorder keeps writing to
// its own nodes after that.
//
// Revisions:
//   - 2026-10-02 08:37: initial creation, as TestThreads_ASnapshotStaysAsItWas
//   - 2026-10-02 15:34: reads a function's node
func TestDrawn_StaysAsItWas(t *testing.T) {
	into := _Begun()

	into.Started(SPINE, &scheduler.Line{Name: "sign"})

	shot := into._Drawn()

	into.Ended(SPINE, "sign", errors.New(BROKE))

	for _, node := range shot.GetFunctions() {
		if node.GetStatus() != workflowpb.Status_STATUS_RUNNING {
			t.Fatalf("want the graph left running, got %v", node)
		}
	}
}

// _Told is every operation a recorder told its watcher, one line each: the op,
// the path, and the value as compact JSON when there is one.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func _Told(t *testing.T, into *_Recorder) *[]string {
	t.Helper()

	told := new([]string)

	into.watch = func(change *workflowpb.Change) {
		for _, op := range change.GetOperations() {
			line := op.GetOp() + " " + op.GetPath()

			if op.GetValue() != nil {
				raw, err := json.Marshal(op.GetValue().AsInterface())
				if err != nil {
					t.Fatalf("value of %s: %v", op.GetPath(), err)
				}

				line += " " + string(raw)
			}

			*told = append(*told, line)
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
func TestMembers_AreTheSchemasJSONNames(t *testing.T) {
	graph := (&workflowpb.Graph{}).ProtoReflect().Descriptor().Fields()
	node := (&workflowpb.Node{}).ProtoReflect().Descriptor().Fields()

	for member, field := range map[string]protoreflect.FieldDescriptor{
		FUNCTIONS: graph.ByName(FUNCTIONS),
		CALLS:     graph.ByName(CALLS),
		STATUS:    graph.ByName(STATUS),
		CAUSE:     graph.ByName(CAUSE),
		THREADS:   node.ByName(THREADS),
	} {
		if field == nil || field.JSONName() != member {
			t.Fatalf("no field written as %q", member)
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
func TestChanges_SayWhatEachStepDid(t *testing.T) {
	into := _NewRecorder()
	told := _Told(t, into)

	into.Started(SPINE, &scheduler.Line{Name: spelling.ENTRY})
	into.Started(SPINE, &scheduler.Line{Name: "fetch"})
	into.Ended(SPINE, "fetch", nil)
	into.Ended(SPINE, spelling.ENTRY, nil)

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

	if strings.Join(*told, "\n") != strings.Join(want, "\n") {
		t.Fatalf("told\n%s\nwant\n%s", strings.Join(*told, "\n"), strings.Join(want, "\n"))
	}
}

// TestChanges_ShowWhichThreadExecutesWhat checks two threads executing one
// function are both on its node, in the order they arrived, and that one
// leaving takes only itself off.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func TestChanges_ShowWhichThreadExecutesWhat(t *testing.T) {
	into := _Begun()

	into.Started(SPINE, &scheduler.Line{Name: "work", Builtin: spelling.SPAWN, Child: LANE})
	into.Started(SPINE, &scheduler.Line{Name: "work", Builtin: spelling.SPAWN, Child: OTHER})

	threads := fmt.Sprint(_Function(into, "work").GetThreads())
	if threads != fmt.Sprint([]string{LANE, OTHER}) {
		t.Fatalf("want both threads on work, got %s", threads)
	}

	into.Ended(LANE, "work", nil)

	threads = fmt.Sprint(_Function(into, "work").GetThreads())
	if threads != fmt.Sprint([]string{OTHER}) {
		t.Fatalf("want only %s left on work, got %s", OTHER, threads)
	}

	spine := fmt.Sprint(_Function(into, spelling.ENTRY).GetThreads())
	if spine != fmt.Sprint([]string{SPINE}) {
		t.Fatalf("want the spine still on main, got %s", spine)
	}
}
