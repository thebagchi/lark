package observe_test

import (
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/observe"
)

// PHANTOM is a function no script defines, used to ask what a graph that is
// wrong about a script does.
const (
	// PHANTOM is a function no script defines.
	PHANTOM = "never-written"

	// WRAPPED_LANE is a thread a graph runs a wrapper on, and EMPTY_LANE one it
	// forks and never describes. Both above any the samples use.
	WRAPPED_LANE = 7
	EMPTY_LANE   = 9

	// THRICE spawns three times where a graph declares two lanes.
	THRICE = "testdata/thrice.star"
)

// _Graph declares the branching script: its four functions, on the spine.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func _Graph(names ...string) *workflowpb.Graph {
	graph := &workflowpb.Graph{}

	for _, name := range names {
		graph.Functions = append(graph.Functions, &workflowpb.Function{Name: name})
	}

	return graph
}

// TestGraph_MakesAnUnreachedFunctionPending is what supplying a graph buys, and
// the only thing it buys.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestGraph_MakesAnUnreachedFunctionPending(t *testing.T) {
	declared := _Graph("alpha", "beta", UNREACHED, "main")

	snap := _Ran(t, BRANCHING, observe.WithGraph(declared))

	node, _ := _Node(snap, UNREACHED)
	if node == nil {
		t.Fatal("want a declared function reported even though nothing called it")
	}

	if node.GetStatus() != workflowpb.Status_STATUS_PENDING {
		t.Fatalf("want it pending, got %v", node.GetStatus())
	}
}

// TestGraph_LeavesAFinishedRunFinished is the row written because it fails
// against the code this phase was going to lift.
//
// .poc/go/status/ folds every node's status into the run's, so any pending node
// makes the run pending. A run that succeeded without reaching two functions
// would then call itself pending for ever, because a pending node has nothing
// further to happen to it. A run's status is the run's own.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestGraph_LeavesAFinishedRunFinished(t *testing.T) {
	declared := _Graph("alpha", "beta", UNREACHED, "main")

	snap := _Ran(t, BRANCHING, observe.WithGraph(declared))

	if snap.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want a finished run to report how it finished, got %v", snap.GetStatus())
	}

	node, _ := _Node(snap, UNREACHED)
	if node.GetStatus() != workflowpb.Status_STATUS_PENDING {
		t.Fatal("want the unreached function still pending after the run ended")
	}
}

// TestGraph_CannotHideAThread is the floor rule: a graph may add what has not
// happened and may never suppress what did.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestGraph_CannotHideAThread(t *testing.T) {
	// A graph that mentions one of the two functions the script spawns.
	declared := _Graph("alpha")

	snap := _Ran(t, BRANCHING, observe.WithGraph(declared))

	for _, name := range []string{"alpha", "beta", "main"} {
		node, _ := _Node(snap, name)
		if node == nil {
			t.Fatalf("want %s reported although the graph did not name it", name)
		}
	}
}

// TestGraph_KeepsAFunctionThatDoesNotExistPending records what a graph wrong
// about a script does: nothing is refused, and the claim stays visible.
//
// A refusal at Start would be a complaint about a file this package never read,
// made at run time. Pending for ever is visibly odd, which is more use.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestGraph_KeepsAFunctionThatDoesNotExistPending(t *testing.T) {
	snap := _Ran(t, BRANCHING, observe.WithGraph(_Graph(PHANTOM)))

	node, _ := _Node(snap, PHANTOM)
	if node == nil {
		t.Fatal("want a graph's claim kept rather than dropped")
	}

	if node.GetStatus() != workflowpb.Status_STATUS_PENDING {
		t.Fatalf("want it pending, got %v", node.GetStatus())
	}
}

// TestGraph_NilBehavesAsNone records that an option with nothing in it is not
// an error a caller has to check.
//
// Revisions:
//   - 2026-09-20 01:42: initial creation
func TestGraph_NilBehavesAsNone(t *testing.T) {
	node, _ := _Node(_Ran(t, BRANCHING, observe.WithGraph(nil)), UNREACHED)

	if node != nil {
		t.Fatalf("want a nil graph to buy nothing, got %v", node.GetStatus())
	}
}

// _Lanes is index+1 authored threads, with steps on that slot. Graph
// threads have no index field; identity is list position, so a function
// that must not sit on the spine needs empty slots in front.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
func _Lanes(index int32, steps []*workflowpb.Step) []*workflowpb.GraphThread {
	lanes := make([]*workflowpb.GraphThread, index+1)

	for slot := range lanes {
		lanes[slot] = new(workflowpb.GraphThread)
	}

	lanes[index].Steps = steps

	return lanes
}

// _Placed builds a graph that runs each named function on its own lane,
// after an empty spine so slot 0 stays the entry thread.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
//   - 2026-09-20 18:40: GraphThread has no index; pads a spine slot
func _Placed(names ...string) *workflowpb.Graph {
	graph := &workflowpb.Graph{}
	graph.Threads = append(graph.Threads, new(workflowpb.GraphThread))

	for _, name := range names {
		graph.Functions = append(graph.Functions, &workflowpb.Function{Name: name})

		graph.Threads = append(graph.Threads, &workflowpb.GraphThread{
			Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{Call: &workflowpb.Call{Function: name}}},
			},
		})
	}

	return graph
}

// TestGraph_PlacesAFunctionOnItsOwnLane is sub-phase 7.1: a graph that says
// where a function runs is believed, and the function is not also entered on
// the spine.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func TestGraph_PlacesAFunctionOnItsOwnLane(t *testing.T) {
	snap := _Ran(t, BRANCHING, observe.WithGraph(_Placed(UNREACHED)))

	node, lane := _Node(snap, UNREACHED)
	if node == nil {
		t.Fatal("want the placed function reported")
	}

	if lane == SPINE {
		t.Fatalf("want it on the lane the graph runs it on, got the spine")
	}

	if _Count(snap, UNREACHED) != 1 {
		t.Fatalf("want one node for it, got %d", _Count(snap, UNREACHED))
	}
}

// TestGraph_DeclaredButUnplacedGoesToTheSpine records the other half: a
// function a graph names and never runs anywhere still appears.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func TestGraph_DeclaredButUnplacedGoesToTheSpine(t *testing.T) {
	snap := _Ran(t, BRANCHING, observe.WithGraph(_Graph(UNREACHED)))

	node, lane := _Node(snap, UNREACHED)
	if node == nil {
		t.Fatal("want a declared function reported")
	}

	if lane != SPINE {
		t.Fatalf("want an unplaced function on the spine, got thread %d", lane)
	}
}

// TestGraph_AWrapperPlacesItsFunction records that a Repeat runs its function
// as surely as a Call does, so where the graph puts the wrapper is where the
// function belongs.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func TestGraph_AWrapperPlacesItsFunction(t *testing.T) {
	declared := &workflowpb.Graph{
		Functions: []*workflowpb.Function{{Name: UNREACHED}},
		Threads: _Lanes(WRAPPED_LANE, []*workflowpb.Step{{
			Action: &workflowpb.Step_Repeat{
				Repeat: &workflowpb.Repeat{Function: UNREACHED, Count: 2},
			},
		}}),
	}

	_, lane := _Node(_Ran(t, BRANCHING, observe.WithGraph(declared)), UNREACHED)

	if lane != WRAPPED_LANE {
		t.Fatalf("want the wrapper's lane %d, got %d", WRAPPED_LANE, lane)
	}
}

// TestGraph_AForkedLaneWithNothingOnItIsStillALane is what phase 7 means by a
// lane a run never fills, and a user interface draws greyed.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func TestGraph_AForkedLaneWithNothingOnItIsStillALane(t *testing.T) {
	declared := &workflowpb.Graph{
		Threads: []*workflowpb.GraphThread{{
			Steps: []*workflowpb.Step{{
				Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: EMPTY_LANE}},
			}},
		}},
	}

	snap := _Ran(t, BRANCHING, observe.WithGraph(declared))

	for _, lane := range snap.GetThreads() {
		if lane.GetIndex() != EMPTY_LANE {
			continue
		}

		if len(lane.GetNodes()) != 0 {
			t.Fatalf("want an empty lane, got %d nodes", len(lane.GetNodes()))
		}

		return
	}

	t.Fatalf("want a lane %d the graph forked and never described", EMPTY_LANE)
}

// AUTHORED is how many lanes the graph in the numbering test declares: the
// spine and two forks. A third spawn runs past them.
const AUTHORED = 3

// TestGraph_LiveNumberingFollowsAuthoredSlots is the claim .doc/workflow.md §9
// makes about a snapshot, and §4 about its nodes, checked end to end.
//
// It is the assumption everything else here rests on. A graph places a function
// on a lane by its list slot; the scheduler numbers threads as it meets them.
// If those two disagree, every placed name belongs to a different thread than
// the one running it, and nothing else in this file would notice - each of the
// other tests uses a graph and a script that agree by construction.
//
// The script spawns three times where the graph declares two lanes, so it also
// covers the other half of §9: a spawn past the authored list is numbered by
// the scheduler, and lands after them rather than colliding.
//
// §4's claim is the shape of the result: first on thread 1 and first on thread
// 3 are two nodes with one name, which is why status lives on Node and not on
// Function.
//
// Revisions:
//   - 2026-09-20 19:40: initial creation
func TestGraph_LiveNumberingFollowsAuthoredSlots(t *testing.T) {
	snap := _Ran(t, THRICE, observe.WithGraph(_Authored()))

	if len(snap.GetThreads()) != AUTHORED+1 {
		t.Fatalf("want %d lanes, got %d", AUTHORED+1, len(snap.GetThreads()))
	}

	want := map[int32]string{0: ENTRY, 1: "first", 2: "second", 3: "first"}

	for _, lane := range snap.GetThreads() {
		nodes := lane.GetNodes()

		if len(nodes) != 1 {
			t.Fatalf("thread %d: want one node, got %d", lane.GetIndex(), len(nodes))
		}

		if nodes[0].GetFunction() != want[lane.GetIndex()] {
			t.Fatalf("thread %d: want %s, got %s",
				lane.GetIndex(), want[lane.GetIndex()], nodes[0].GetFunction())
		}
	}

	// §4: one name, two nodes. Status is a property of a thread's run of a
	// function, not of the function.
	if _Count(snap, "first") != 2 {
		t.Fatalf("want first reported twice, got %d", _Count(snap, "first"))
	}
}

// _Authored is a graph declaring the spine, first on lane 1 and second on lane
// 2 - two lanes for a script that spawns three times.
//
// Revisions:
//   - 2026-09-20 19:40: initial creation
func _Authored() *workflowpb.Graph {
	return &workflowpb.Graph{
		Functions: []*workflowpb.Function{
			{Name: "first"},
			{Name: "second"},
			{Name: ENTRY},
		},
		Threads: []*workflowpb.GraphThread{
			{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{Call: &workflowpb.Call{Function: ENTRY}}},
				{Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: 1}}},
				{Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: 2}}},
			}},
			{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{Call: &workflowpb.Call{Function: "first"}}},
			}},
			{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{Call: &workflowpb.Call{Function: "second"}}},
			}},
		},
	}
}
