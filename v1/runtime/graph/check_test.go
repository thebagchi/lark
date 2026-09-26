package graph_test

import (
	"errors"
	"strings"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

// _Authored is a graph of one spine and the threads given.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 23:53: the spine names the entry point in its first step
func _Authored(spine []*workflowpb.Step, rest ...*workflowpb.Thread) *workflowpb.Graph {
	steps := append([]*workflowpb.Step{_CallOf(graph.ENTRY)}, spine...)

	threads := []*workflowpb.Thread{{
		Id:    graph.SPINE,
		State: &workflowpb.Thread_Static{Static: &workflowpb.Static{Steps: steps}},
	}}

	return &workflowpb.Graph{Threads: append(threads, rest...)}
}

// _Runs is an authored thread under this id, running this function.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 23:53: says what it runs in its first step
func _Runs(id string, name string) *workflowpb.Thread {
	return &workflowpb.Thread{
		Id: id,
		State: &workflowpb.Thread_Static{
			Static: &workflowpb.Static{Steps: []*workflowpb.Step{_CallOf(name)}},
		},
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

// TestCheck_RefusesTwoFunctionsOfOneName is the check that used to be reachable
// only by calling Distinct as well, which nothing said to do.
//
// Revisions:
//   - 2026-09-21 16:25: initial creation
func TestCheck_RefusesTwoFunctionsOfOneName(t *testing.T) {
	built := _Authored(nil)
	built.Functions = []*workflowpb.Function{{Name: GREET}, {Name: GREET}}

	err := graph.Check(built)
	if !errors.Is(err, graph.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}

	if !strings.Contains(err.Error(), GREET) {
		t.Fatalf("want the name in the refusal, got %v", err)
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

// TestCheck_RefusesTwoEntryPoints records that the spine is the thread whose
// id says so, and there is one of those.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 23:53: two threads claiming the spine's id, since an entry is
//     no longer a field to omit
func TestCheck_RefusesTwoEntryPoints(t *testing.T) {
	built := _Authored(nil, _Runs(graph.SPINE, graph.ENTRY))

	if err := graph.Check(built); !errors.Is(err, graph.ErrTwoSpines) {
		t.Fatalf("want ErrTwoSpines, got %v", err)
	}
}

// TestCheck_RefusesAThreadThatNamesNothing is what a thread with no steps is:
// one that does not say what it runs, so nothing can be generated for it.
//
// Revisions:
//   - 2026-09-21 23:53: initial creation
func TestCheck_RefusesAThreadThatNamesNothing(t *testing.T) {
	built := _Authored(nil, &workflowpb.Thread{
		Id:    "thread_1",
		State: &workflowpb.Thread_Static{Static: new(workflowpb.Static)},
	})

	if err := graph.Check(built); !errors.Is(err, graph.ErrNoEntry) {
		t.Fatalf("want ErrNoEntry, got %v", err)
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
		Steps: []*workflowpb.Step{_CallOf("alpha"), _Fork("thread_9_4")},
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

// TestCheck_RefusesASpineThatForksItself is the hole the spine's own arm had.
//
// The spine contributes no prefix, so its children are thread_1 rather than
// thread_0_1 - and the test for that was "begins with thread_ and the ordinal
// holds no underscore", which thread_0 satisfies. A fork of thread_0 declared
// on thread_0 therefore read as an ordinary child, and a graph whose spine
// forks itself passed. The general arm never had the hole, because thread_1
// does not begin with "thread_1_".
//
// Revisions:
//   - 2026-09-24 16:38: initial creation
func TestCheck_RefusesASpineThatForksItself(t *testing.T) {
	built := _Authored([]*workflowpb.Step{_Fork(graph.SPINE)})

	err := graph.Check(built)
	if !errors.Is(err, graph.ErrParentage) {
		t.Fatalf("a spine forking itself gave %v, want ErrParentage", err)
	}

	if !strings.Contains(err.Error(), graph.SPINE) {
		t.Fatalf("the refusal does not name the thread: %v", err)
	}
}

// TestCheck_RefusesAnIdWithNoOrdinal is the neighbour of the self-fork, and it
// is not caught by the same guard.
//
// thread_ is not equal to thread_0, so "nothing descends from itself" does not
// reach it - and an empty ordinal contains no underscore, so the spine's arm
// read it as an ordinary child. An id that names no ordinal names no thread.
//
// The odd spellings next to it were waved for a day and are now refused too:
// an ordinal is a decimal counting from one, so thread_01, thread_00 and
// thread_1a name threads the scheduler would never mint. That started as a
// rule about parentage and is now also a rule about how an ordinal is spelled,
// which was asked for after the first version of this test recorded the
// opposite.
//
// Revisions:
//   - 2026-09-24 16:44: initial creation
//   - 2026-09-24 17:12: the odd spellings flip from accepted to refused
func TestCheck_RefusesAnIdWithNoOrdinal(t *testing.T) {
	built := _Authored([]*workflowpb.Step{_Fork(graph.THREAD)}, _Runs(graph.THREAD, "alpha"))

	err := graph.Check(built)
	if !errors.Is(err, graph.ErrParentage) {
		t.Fatalf("a fork of %q gave %v, want ErrParentage", graph.THREAD, err)
	}

	for _, odd := range []string{"thread_01", "thread_00", "thread_1a"} {
		t.Run(odd, func(t *testing.T) {
			held := _Authored([]*workflowpb.Step{_Fork(odd)}, _Runs(odd, "alpha"))

			if !errors.Is(graph.Check(held), graph.ErrParentage) {
				t.Fatalf("%q was accepted, and no ordinal is spelled that way", odd)
			}
		})
	}

	// And the ones the scheduler does mint stay legal.
	for _, good := range []string{"thread_1", "thread_10"} {
		t.Run(good, func(t *testing.T) {
			held := _Authored([]*workflowpb.Step{_Fork(good)}, _Runs(good, "alpha"))

			if err := graph.Check(held); err != nil {
				t.Fatalf("%q was refused: %v", good, err)
			}
		})
	}
}

// TestCheck_KeepsTheForksThatAreLegal checks the fix refused only what it
// should: the spine's real children still pass, and a grandchild declared on
// the spine still fails.
//
// Revisions:
//   - 2026-09-24 16:38: initial creation
func TestCheck_KeepsTheForksThatAreLegal(t *testing.T) {
	legal := _Authored([]*workflowpb.Step{_Fork("thread_1")}, _Runs("thread_1", "alpha"))

	err := graph.Check(legal)
	if err != nil {
		t.Fatalf("the spine forking thread_1 gave %v, want it allowed", err)
	}

	skipped := _Authored([]*workflowpb.Step{_Fork("thread_1_1")}, _Runs("thread_1_1", "alpha"))

	err = graph.Check(skipped)
	if !errors.Is(err, graph.ErrParentage) {
		t.Fatalf("the spine forking a grandchild gave %v, want ErrParentage", err)
	}
}
