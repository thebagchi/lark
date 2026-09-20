package graph_test

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/graph"
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
		Threads: []*workflowpb.GraphThread{
			_Spine(
				_Fork(1),
				_Fork(2),
				_JoinOf(1, 2),
			),
			_Lane("first"),
			_Lane("second"),
		},
	}
}

// _Spine is thread 0: its own call, then the steps given.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func _Spine(steps ...*workflowpb.Step) *workflowpb.GraphThread {
	return &workflowpb.GraphThread{Steps: append([]*workflowpb.Step{_CallOf("main")}, steps...)}
}

// _Lane is a thread that names one function and says nothing more.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func _Lane(name string, args ...*structpb.Value) *workflowpb.GraphThread {
	return &workflowpb.GraphThread{Steps: []*workflowpb.Step{_CallOf(name, args...)}}
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

// _Fork names a slot.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func _Fork(slot int32) *workflowpb.Step {
	return &workflowpb.Step{Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: slot}}}
}

// _JoinOf names the slots it waits for.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func _JoinOf(slots ...int32) *workflowpb.Step {
	return &workflowpb.Step{
		Action: &workflowpb.Step_Join{Join: &workflowpb.Join{Threads: slots}},
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

	if strings.Contains(string(out), "spawn(1)") {
		t.Fatal("want the slot's function, not the slot")
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
	built.Threads[1] = _Lane("first", structpb.NewStringValue("alice"))

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

// TestSteps_RefusesASlotThatIsNotThere is the refusal a UI reaches by building
// a graph wrong rather than by writing a bad body.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func TestSteps_RefusesASlotThatIsNotThere(t *testing.T) {
	built := CONCURRENT()
	built.Threads[0] = _Spine(_Fork(9))

	_, err := graph.Emit(built)
	if !errors.Is(err, graph.ErrNoSlot) {
		t.Fatalf("want ErrNoSlot, got %v", err)
	}

	if !strings.Contains(err.Error(), "9") {
		t.Fatalf("want the slot named, got %v", err)
	}
}

// TestSteps_TheScriptRuns is the phase. Everything above checks text; this
// checks that the text is a program that does what the graph describes.
//
// Revisions:
//   - 2026-09-20 21:04: initial creation
func TestSteps_TheScriptRuns(t *testing.T) {
	built := CONCURRENT()
	built.Threads[0] = _Spine(_Fork(1), _Fork(2), _JoinOf(1, 2), _CallOf("total"))
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
