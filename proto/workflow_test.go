// Package workflow_test exercises the authored Graph JSON: Function body and
// params, Call args, and a Thread that carries its own id and names what runs
// on it.
package workflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

const (
	// ENTRY is the function every script starts at.
	ENTRY = "main"

	// BODY is the statements inside def greet(name).
	BODY = `return "hello " + name`

	// TODAY is a name-only graph as measured before this phase.
	TODAY = `{"functions":[{"name":"first"},{"name":"second"},{"name":"main"}],` +
		`"threads":[{"id":"thread_0","static":{"steps":[` +
		`{"call":{"function":"main"}},` +
		`{"fork":{"thread":"thread_1"}},` +
		`{"fork":{"thread":"thread_2"}},` +
		`{"join":{"threads":["thread_1","thread_2"]}}]}},` +
		`{"id":"thread_1","static":{"steps":[{"call":{"function":"first"}}]}},` +
		`{"id":"thread_2","static":{"steps":[{"call":{"function":"second"}}]}}]}`
)

// TestFunction_CarriesBodyAndParams is the draft a UI sends: a chatbot
// fills body, the UI names the parameters, and there is no status.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
func TestFunction_CarriesBodyAndParams(t *testing.T) {
	raw, err := protojson.Marshal(&workflowpb.Function{
		Name:   "greet",
		Body:   BODY,
		Params: []string{"name"},
	})
	if err != nil {
		t.Fatal(err)
	}

	text := string(raw)
	if !strings.Contains(text, `"body"`) {
		t.Fatalf("want a body the chatbot can fill, got %s", text)
	}

	if !strings.Contains(text, `"params"`) {
		t.Fatalf("want parameter names on the function, got %s", text)
	}

	if strings.Contains(text, `"status"`) {
		t.Fatalf("Function has no status field: %s", text)
	}
}

// TestCall_ArgsKeepJsonKinds is option C: a string, a number and an
// object round-trip as JSON, not as three strings.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
func TestCall_ArgsKeepJsonKinds(t *testing.T) {
	list, err := structpb.NewList([]any{
		"alice",
		3,
		map[string]any{"k": 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := protojson.Marshal(&workflowpb.Call{
		Function: "greet",
		Args:     list.GetValues(),
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("call: %s", raw)

	var site map[string]any

	if err := json.Unmarshal(raw, &site); err != nil {
		t.Fatal(err)
	}

	args, ok := site["args"].([]any)
	if !ok || len(args) != 3 {
		t.Fatalf("want three args, got %s", raw)
	}

	if args[0] != "alice" {
		t.Fatalf("want a string, got %#v", args[0])
	}

	n, ok := args[1].(float64)
	if !ok || n != 3 {
		t.Fatalf("want the number 3, got %#v", args[1])
	}

	obj, ok := args[2].(map[string]any)
	if !ok || obj["k"] != float64(1) {
		t.Fatalf("want an object, got %#v", args[2])
	}
}

// TestGraph_NameOnlyStillDecodes is today's JSON after the new fields:
// empty body, empty args, no index on authored threads.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
func TestGraph_NameOnlyStillDecodes(t *testing.T) {
	graph := new(workflowpb.Graph)
	if err := protojson.Unmarshal([]byte(TODAY), graph); err != nil {
		t.Fatal(err)
	}

	if len(graph.GetFunctions()) != 3 {
		t.Fatalf("want three functions, got %d", len(graph.GetFunctions()))
	}

	for _, fn := range graph.GetFunctions() {
		if fn.GetBody() != "" {
			t.Fatalf("want today's body empty, got %q", fn.GetBody())
		}

		if len(fn.GetParams()) != 0 {
			t.Fatalf("want today's params empty, got %v", fn.GetParams())
		}
	}

	spine := graph.GetThreads()[0].GetStatic().GetSteps()
	if spine[0].GetCall().GetFunction() != "main" {
		t.Fatalf("want the spine to run main in its first step, got %v", spine[0])
	}

	call := graph.GetThreads()[1].GetStatic().GetSteps()[0].GetCall()
	if call.GetFunction() != "first" {
		t.Fatalf("want thread_1 to run first, got %s", call.GetFunction())
	}

	if len(call.GetArgs()) != 0 {
		t.Fatalf("want today's args empty, got %v", call.GetArgs())
	}
}

// TestThread_NamesItsParent is what a list position could not carry: an id
// that says whose child a thread is.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation, as TestGraphThread_HasNoIndex
//   - 2026-09-21 00:59: a thread carries an id, reversing .doc/workflow.md
//     §9, because a hierarchical id states parentage and a slot cannot
//   - 2026-09-21 23:53: the first step is what the thread runs, so the fork
//     it makes is the one after
func TestThread_NamesItsParent(t *testing.T) {
	graph := new(workflowpb.Graph)
	if err := protojson.Unmarshal([]byte(TODAY), graph); err != nil {
		t.Fatal(err)
	}

	// Past the first, which is what the spine itself runs.
	named := graph.GetThreads()[0].GetStatic().GetSteps()[1].GetFork().GetThread()
	if named != "thread_1" {
		t.Fatalf("want the spawn to name thread_1, got %s", named)
	}

	if graph.GetThreads()[1].GetId() != named {
		t.Fatalf("want a thread under %s, got %s", named, graph.GetThreads()[1].GetId())
	}
}

// TestThread_RefusesNodesOnAnAuthoredGraph is what the oneof buys over a
// single flat message: a graph cannot carry progress, by type rather than
// by convention.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func TestThread_RefusesNodesOnAnAuthoredGraph(t *testing.T) {
	raw := []byte(`{"id":"thread_1","static":{"nodes":[{"function":"first"}]}}`)

	opts := protojson.UnmarshalOptions{DiscardUnknown: false}

	lane := new(workflowpb.Thread)
	err := opts.Unmarshal(raw, lane)
	if err == nil {
		t.Fatal("want the static half to refuse nodes")
	}

	if !strings.Contains(err.Error(), "nodes") {
		t.Fatalf("want the refusal to name nodes, got %v", err)
	}
}

// TestThread_NamesOneHalfNeverBoth is what a oneof is for.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func TestThread_NamesOneHalfNeverBoth(t *testing.T) {
	lane := &workflowpb.Thread{
		Id:    "thread_1",
		State: &workflowpb.Thread_Static{Static: new(workflowpb.Static)},
	}

	lane.State = &workflowpb.Thread_Live{Live: new(workflowpb.Live)}

	if lane.GetStatic() != nil {
		t.Fatal("want setting the live half to clear the static one")
	}

	if lane.GetLive() == nil {
		t.Fatal("want the live half set")
	}
}

// TestWorkflow_KeepsItsThreadIds is the live result a UI draws: thread_1
// then thread_1_1, each carrying the id, not the list slot.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation, as TestWorkflow_KeepsIndex
//   - 2026-09-21 00:59: a thread id is a string that names its parent
func TestWorkflow_KeepsItsThreadIds(t *testing.T) {
	snap := &workflowpb.Workflow{
		Status: workflowpb.Status_STATUS_RUNNING,
		Threads: []*workflowpb.Thread{
			_Running("thread_1", "first", workflowpb.Status_STATUS_SUCCEEDED),
			_Running("thread_1_1", "first", workflowpb.Status_STATUS_RUNNING),
		},
	}

	raw, err := protojson.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}

	text := string(raw)
	if !strings.Contains(text, `"id":"thread_1"`) {
		t.Fatalf("want live thread_1, got %s", text)
	}

	if !strings.Contains(text, `"id":"thread_1_1"`) {
		t.Fatalf("want live thread_1_1, got %s", text)
	}

	if snap.GetThreads()[0].GetId() != "thread_1" {
		t.Fatalf("want the first snapshot thread to be thread_1, not list slot 0")
	}
}

// _Running is one live thread, running or having run a single function.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func _Running(id string, name string, status workflowpb.Status) *workflowpb.Thread {
	return &workflowpb.Thread{
		Id: id,
		State: &workflowpb.Thread_Live{
			Live: &workflowpb.Live{
				Nodes: []*workflowpb.Node{{Function: name, Status: status}},
			},
		},
	}
}

// TestGraph_AuthoredJSON is the payload a UI sends after this phase:
// bodies, params, and a spine that names no entry.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
//   - 2026-09-21 00:59: the spine carries no entry, since the entry point is
//     the runtime's rather than the graph's to name
func TestGraph_AuthoredJSON(t *testing.T) {
	graph := &workflowpb.Graph{
		Functions: []*workflowpb.Function{
			{Name: ENTRY, Body: `return greet("world")`},
			{Name: "greet", Params: []string{"name"}, Body: BODY},
		},
		Threads: []*workflowpb.Thread{{
			Id:    "thread_0",
			State: &workflowpb.Thread_Static{Static: new(workflowpb.Static)},
		}},
	}

	raw, err := protojson.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("authored: %s", raw)

	text := string(raw)
	if !strings.Contains(text, `"body"`) {
		t.Fatalf("want bodies, got %s", text)
	}

	if strings.Contains(text, `"entry"`) {
		t.Fatalf("want the spine to name no entry, got %s", text)
	}
}
