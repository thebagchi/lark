// Package workflow_test exercises the authored Graph JSON this phase
// added: Function body and params, Call args, and GraphThread with no
// index.
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
		`"threads":[{"steps":[{"call":{"function":"main"}},` +
		`{"fork":{"thread":1}},{"fork":{"thread":2}},` +
		`{"join":{"threads":[1,2]}}]},` +
		`{"steps":[{"call":{"function":"first"}}]},` +
		`{"steps":[{"call":{"function":"second"}}]}]}`
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

	call := graph.GetThreads()[0].GetSteps()[0].GetCall()
	if call.GetFunction() != ENTRY {
		t.Fatalf("want thread 0 to open with %s, got %s", ENTRY, call.GetFunction())
	}

	if len(call.GetArgs()) != 0 {
		t.Fatalf("want today's args empty, got %v", call.GetArgs())
	}
}

// TestGraphThread_HasNoIndex is why a UI cannot send a thread id:
// authored JSON has steps only. Fork still names list slot 1.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
func TestGraphThread_HasNoIndex(t *testing.T) {
	graph := &workflowpb.Graph{
		Functions: []*workflowpb.Function{
			{Name: "first"},
			{Name: ENTRY},
		},
		Threads: []*workflowpb.GraphThread{
			{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{
					Call: &workflowpb.Call{Function: ENTRY},
				}},
				{Action: &workflowpb.Step_Fork{
					Fork: &workflowpb.Fork{Thread: 1},
				}},
			}},
			{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{
					Call: &workflowpb.Call{Function: "first"},
				}},
			}},
		},
	}

	raw, err := protojson.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}

	text := string(raw)
	if strings.Contains(text, `"index"`) {
		t.Fatalf("want no index on a Graph thread, got %s", text)
	}

	fork := graph.GetThreads()[0].GetSteps()[1].GetFork().GetThread()
	if fork != 1 {
		t.Fatalf("want Fork to name list slot 1, got %d", fork)
	}
}

// TestGraphThread_RefusesIndex is the confusion gone: a UI that still
// puts "index":99 on a Graph thread is refused, because GraphThread
// has no such field.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
func TestGraphThread_RefusesIndex(t *testing.T) {
	raw := []byte(`{"index":99,"steps":[{"call":{"function":"first"}}]}`)

	opts := protojson.UnmarshalOptions{DiscardUnknown: false}

	lane := new(workflowpb.GraphThread)
	err := opts.Unmarshal(raw, lane)
	if err == nil {
		t.Fatal("want GraphThread to refuse index")
	}

	if !strings.Contains(err.Error(), "index") {
		t.Fatalf("want the refusal to name index, got %v", err)
	}
}

// TestWorkflow_KeepsIndex is the live result a UI draws: thread 1
// then thread 3, each carrying the id, not the list slot.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
func TestWorkflow_KeepsIndex(t *testing.T) {
	snap := &workflowpb.Workflow{
		Status: workflowpb.Status_STATUS_RUNNING,
		Threads: []*workflowpb.Thread{
			{
				Index: 1,
				Nodes: []*workflowpb.Node{{
					Function: "first",
					Status:   workflowpb.Status_STATUS_SUCCEEDED,
				}},
			},
			{
				Index: 3,
				Nodes: []*workflowpb.Node{{
					Function: "first",
					Status:   workflowpb.Status_STATUS_RUNNING,
				}},
			},
		},
	}

	raw, err := protojson.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}

	text := string(raw)
	if !strings.Contains(text, `"index":1`) {
		t.Fatalf("want live index 1, got %s", text)
	}

	if !strings.Contains(text, `"index":3`) {
		t.Fatalf("want live index 3, got %s", text)
	}

	if snap.GetThreads()[0].GetIndex() != 1 {
		t.Fatalf("want the first snapshot thread numbered 1, not list slot 0")
	}
}

// TestGraph_AuthoredJSON is the payload a UI sends after this phase:
// bodies, params, and a spine with no index.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
func TestGraph_AuthoredJSON(t *testing.T) {
	graph := &workflowpb.Graph{
		Functions: []*workflowpb.Function{
			{Name: ENTRY, Body: `return greet("world")`},
			{Name: "greet", Params: []string{"name"}, Body: BODY},
		},
		Threads: []*workflowpb.GraphThread{
			{Steps: []*workflowpb.Step{
				{Action: &workflowpb.Step_Call{
					Call: &workflowpb.Call{Function: ENTRY},
				}},
			}},
		},
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

	if strings.Contains(text, `"index"`) {
		t.Fatalf("want no index on authored threads, got %s", text)
	}
}
