package graph_test

import (
	"errors"
	"strings"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/graph"
)

// _Authored is a graph of one spine and the threads given.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Authored(spine []*workflowpb.Step, rest ...*workflowpb.Thread) *workflowpb.Graph {
	threads := []*workflowpb.Thread{{
		Id:    graph.SPINE,
		State: &workflowpb.Thread_Static{Static: &workflowpb.Static{Steps: spine}},
	}}

	return &workflowpb.Graph{Threads: append(threads, rest...)}
}

// _Runs is an authored thread under this id, running this function.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Runs(id string, name string) *workflowpb.Thread {
	return &workflowpb.Thread{
		Id:    id,
		Entry: &workflowpb.Call{Function: name},
		State: &workflowpb.Thread_Static{Static: new(workflowpb.Static)},
	}
}

// TestCheck_AcceptsWhatDerivationProduces is the floor: every sample this
// repository ships derives into a graph Check passes.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestCheck_AcceptsWhatDerivationProduces(t *testing.T) {
	for _, name := range _Carried(t) {
		report := _Sample(t, name)

		if err := graph.Check(report.Graph); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// TestCheck_RefusesProgressInAGraph is what the oneof buys: a graph cannot
// carry what a run did.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestCheck_RefusesProgressInAGraph(t *testing.T) {
	built := _Authored(nil)
	built.Threads[0].State = &workflowpb.Thread_Live{Live: new(workflowpb.Live)}

	if err := graph.Check(built); !errors.Is(err, graph.ErrLive) {
		t.Fatalf("want ErrLive, got %v", err)
	}
}

// TestCheck_RefusesTwoEntryPoints records that an unset entry means the
// artifact's entry point, and there is one of those.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestCheck_RefusesTwoEntryPoints(t *testing.T) {
	built := _Authored(nil, &workflowpb.Thread{
		Id:    "thread_1",
		State: &workflowpb.Thread_Static{Static: new(workflowpb.Static)},
	})

	if err := graph.Check(built); !errors.Is(err, graph.ErrTwoSpines) {
		t.Fatalf("want ErrTwoSpines, got %v", err)
	}
}

// TestCheck_RefusesAnIdThatDoesNotNameItsParent is .doc/workflow.md §9's worry
// in the shape a hierarchical id gives it.
//
// §9 removed the id so a user interface could not send one that disagreed with
// list position. An id disagreeing with its parentage is checkable where an
// inconsistent index was not, because an id has a rule and a position does
// not - which is the whole argument for reversing §9.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestCheck_RefusesAnIdThatDoesNotNameItsParent(t *testing.T) {
	deep := _Runs("thread_1", "alpha")
	deep.State = &workflowpb.Thread_Static{Static: &workflowpb.Static{
		Steps: []*workflowpb.Step{_Fork("thread_9_4")},
	}}

	built := _Authored([]*workflowpb.Step{_Fork("thread_1")}, deep, _Runs("thread_9_4", "deep"))

	err := graph.Check(built)
	if !errors.Is(err, graph.ErrParentage) {
		t.Fatalf("want ErrParentage, got %v", err)
	}

	for _, want := range []string{"thread_9_4", "thread_1"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("want both named, got %v", err)
		}
	}
}

// TestCheck_AcceptsTheSpineContributingNoPrefix is the one special case the id
// rule has, checked rather than assumed.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestCheck_AcceptsTheSpineContributingNoPrefix(t *testing.T) {
	built := _Authored([]*workflowpb.Step{_Fork("thread_1")}, _Runs("thread_1", "alpha"))

	if err := graph.Check(built); err != nil {
		t.Fatalf("want the spine's child accepted, got %v", err)
	}

	// And thread_0_1 is not how the spine names a child.
	wrong := _Authored([]*workflowpb.Step{_Fork("thread_0_1")}, _Runs("thread_0_1", "alpha"))

	if err := graph.Check(wrong); !errors.Is(err, graph.ErrParentage) {
		t.Fatalf("want thread_0_1 refused, got %v", err)
	}
}

// TestCheck_RefusesACallThatCannotFit turns a run-time Starlark error into a
// refusal that names the call.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestCheck_RefusesACallThatCannotFit(t *testing.T) {
	built := _Authored([]*workflowpb.Step{_CallOf("greet", _Text("a"), _Text("b"))})
	built.Functions = []*workflowpb.Function{{Name: "greet", Params: []string{"who"}}}

	err := graph.Check(built)
	if !errors.Is(err, graph.ErrArity) {
		t.Fatalf("want ErrArity, got %v", err)
	}

	if !strings.Contains(err.Error(), "greet") {
		t.Fatalf("want the call named, got %v", err)
	}
}
