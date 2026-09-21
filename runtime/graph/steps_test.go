package graph_test

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/artifact"
	"github.com/thebagchi/lark/runtime/graph"
	"github.com/thebagchi/lark/runtime/plugin/core"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// CONCURRENT is the graph .doc/workflow.md shows: a spine that forks two
// threads and joins them, and two leaves that say what they return.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func CONCURRENT() *workflowpb.Graph {
	return &workflowpb.Graph{
		Functions: []*workflowpb.Function{
			_Fn("first", "return 1"),
			_Fn("second", "return 2"),
			_Fn("main", ""),
		},
		Threads: []*workflowpb.Thread{
			_Spine(
				_Fork("thread_1"),
				_Fork("thread_2"),
				_JoinOf("thread_1", "thread_2"),
			),
			_Lane("thread_1", "first"),
			_Lane("thread_2", "second"),
		},
	}
}

// _Spine is the entry point's thread: no entry of its own, and the steps given.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
//   - 2026-09-21 00:59: carries no head Call, since the entry point is the
//     runtime's to name
func _Spine(steps ...*workflowpb.Step) *workflowpb.Thread {
	return &workflowpb.Thread{
		Id:    "thread_0",
		State: &workflowpb.Thread_Static{Static: &workflowpb.Static{Steps: steps}},
	}
}

// _Lane is a thread that names one function and says nothing more.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
//   - 2026-09-21 00:59: names its function through its entry, under its own id
func _Lane(id string, name string, args ...*structpb.Value) *workflowpb.Thread {
	return &workflowpb.Thread{
		Id:    id,
		Entry: &workflowpb.Call{Function: name, Args: args},
		State: &workflowpb.Thread_Static{Static: new(workflowpb.Static)},
	}
}

// _CallOf, _Fork and _JoinOf are the three steps this phase renders.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func _CallOf(name string, args ...*structpb.Value) *workflowpb.Step {
	return &workflowpb.Step{
		Action: &workflowpb.Step_Call{Call: &workflowpb.Call{Function: name, Args: args}},
	}
}

// _Fork names the thread it starts.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
//   - 2026-09-21 01:32: a thread id is a string
func _Fork(id string) *workflowpb.Step {
	return &workflowpb.Step{
		Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: id}},
	}
}

// _JoinOf names the threads it waits for.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
//   - 2026-09-21 01:32: a thread id is a string
func _JoinOf(ids ...string) *workflowpb.Step {
	return &workflowpb.Step{
		Action: &workflowpb.Step_Join{Join: &workflowpb.Join{Threads: ids}},
	}
}

// TestSteps_TheConcurrentGraphGeneratesItsScript is phase 5, byte for byte.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func TestSteps_TheConcurrentGraphGeneratesItsScript(t *testing.T) {
	out, err := graph.Emit(CONCURRENT())
	if err != nil {
		t.Fatal(err)
	}

	want := "def main():\n    h1 = spawn(first)\n    h2 = spawn(second)\n    join(h1, h2)\n    pass\n"

	if !strings.Contains(string(out), want) {
		t.Fatalf("want\n%q\ngot\n%q", want, out)
	}
}

// TestSteps_ALeafKeepsItsBody is the rule a first implementation got wrong: a
// thread that only names a function is a fork pointing at a leaf, not a
// description of what that leaf does.
//
// Generating from it drops the authored body and emits an empty function.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func TestSteps_ALeafKeepsItsBody(t *testing.T) {
	out, err := graph.Emit(CONCURRENT())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"def first():\n    return 1\n", "def second():\n    return 2\n"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("want %q kept, got\n%s", want, out)
		}
	}
}

// TestSteps_AForkRendersTheSlotsFunction records the indirection: a fork names
// a slot, and what runs there is that slot's own first Call.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func TestSteps_AForkRendersTheSlotsFunction(t *testing.T) {
	out, err := graph.Emit(CONCURRENT())
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(out), `spawn("thread_1")`) {
		t.Fatal("want the thread's function, not the thread id")
	}
}

// TestSteps_ASiteWithArgumentsIsALambda is the decision of 2026-09-20 20:06
// rendered: a lambda appears only where a call passes arguments.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func TestSteps_ASiteWithArgumentsIsALambda(t *testing.T) {
	built := CONCURRENT()
	built.Functions[0] = _Fn("first", "return who", "who")
	built.Threads[1] = _Lane("thread_1", "first", structpb.NewStringValue("alice"))

	out, err := graph.Emit(built)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), `h1 = spawn(lambda: first("alice"))`) {
		t.Fatalf("want a lambda for a site with arguments, got\n%s", out)
	}

	// And the one without them is still the function itself.
	if !strings.Contains(string(out), "h2 = spawn(second)") {
		t.Fatalf("want no lambda where there are no arguments, got\n%s", out)
	}
}

// TestSteps_RefusesAThreadThatIsNotThere is the refusal a UI reaches by
// building a graph wrong rather than by writing a bad body.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation, as TestSteps_RefusesASlotThatIsNotThere
//   - 2026-09-21 00:59: a spawn names a thread id, so the refusal names one
func TestSteps_RefusesAThreadThatIsNotThere(t *testing.T) {
	built := CONCURRENT()
	built.Threads[0] = _Spine(_Fork("thread_9"))

	_, err := graph.Emit(built)
	if !errors.Is(err, graph.ErrNoThread) {
		t.Fatalf("want ErrNoThread, got %v", err)
	}

	if !strings.Contains(err.Error(), "thread_9") {
		t.Fatalf("want the thread named, got %v", err)
	}
}

// TestSteps_TheScriptRuns is the phase. Everything above checks text; this
// checks that the text is a program that does what the graph describes.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func TestSteps_TheScriptRuns(t *testing.T) {
	built := CONCURRENT()
	built.Threads[0] = _Spine(
		_Fork("thread_1"),
		_Fork("thread_2"),
		_JoinOf("thread_1", "thread_2"),
		_CallOf("total"),
	)
	built.Functions = append(built.Functions, _Fn("total", "return 3"))

	out, err := graph.Emit(built)
	if err != nil {
		t.Fatal(err)
	}

	art, err := runtime.NewCompiler().Compile(SCRIPT, out)
	if err != nil {
		t.Fatalf("want a compiling script, got %v\n%s", err, out)
	}

	_, err = art.Run(t.Context())
	if err != nil {
		t.Fatalf("want it to run, got %v\n%s", err, out)
	}
}

// TestSteps_TheNamesMatchTheRuntime pins the five strings this package repeats
// rather than imports.
//
// Generating a script must not depend on compiling or running one, so the
// entry point's name, the spine's id and the three thread builtins are
// declared here as well as where they are defined. That is a duplication, and this is what stops it
// drifting: a rename on either side fails here rather than producing a script
// that calls something nothing registers.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func TestSteps_TheNamesMatchTheRuntime(t *testing.T) {
	cases := []struct {
		mine   string
		theirs string
		what   string
	}{
		{graph.ENTRY, artifact.ENTRY, "the entry point"},
		{graph.SPINE, scheduler.SPINE, "the spine"},
		{graph.SPAWN, core.SPAWN, "spawn"},
		{graph.JOIN, core.JOIN, "join"},
		{graph.CANCEL, core.CANCEL, "cancel"},
	}

	for _, item := range cases {
		if item.mine != item.theirs {
			t.Fatalf("%s is %q here and %q there", item.what, item.mine, item.theirs)
		}
	}
}
