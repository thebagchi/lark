// Package workflow_test exercises the workflow schema's JSON: a flow as a user
// interface authors it, a graph as a run reports it, and a change to that
// graph as a JSON Patch.
package workflow_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

const (
	// BODY is the statements inside def greet(name), as text.
	BODY = `return "hello " + name`

	// CHANGE is a change as a host is sent it: an RFC 6902 patch, which any
	// JSON Patch library applies to a graph held as JSON. Its operations take
	// a value, none, and a from, so each field is seen present and absent.
	CHANGE = `{"operations": [
		{"op": "add", "path": "/functions/-",
		 "value": {"name": "sign", "status": "STATUS_RUNNING"}},
		{"op": "remove", "path": "/cause"},
		{"op": "move", "from": "/calls/2", "path": "/calls/1"}
	]}`

	// GRAPH is a graph as a user interface reads it: each function with its
	// status, sorted by name, the calls between them, and the cause of the
	// failure that ended the run.
	GRAPH = `{
		"status": "STATUS_FAILED",
		"functions": [
			{"name": "boom", "status": "STATUS_FAILED"},
			{"name": "main", "status": "STATUS_FAILED"},
			{"name": "work", "status": "STATUS_SUCCEEDED"}
		],
		"calls": [
			{"caller": "main", "callee": "boom"},
			{"caller": "main", "callee": "work"}
		],
		"cause": {"function": "boom", "failure": "fail: boom"}
	}`

	// AUTHORED is a flow as a user interface sends it: one function as
	// statements, one as text, and main as statements.
	AUTHORED = `{
		"functions": [
			{"name": "greet", "params": ["name"], "body": "return \"hello \" + name"},
			{"name": "task", "statements": {"statement": [
				{"call": {"function": "greet", "args": ["west"]}}
			]}}
		],
		"main": {"statement": [
			{"spawn": {"binding": "h", "call": {"function": "task"}}},
			{"join": {"bindings": ["h"]}}
		]}
	}`
)

// TestFunction_CarriesBodyOrStatements checks a function travels as text or as
// statements, never both: setting one clears the other.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation, as TestFunction_CarriesBodyAndParams
//   - 2026-10-02 01:46: a body or statements, which a oneof keeps apart
func TestFunction_CarriesBodyOrStatements(t *testing.T) {
	fn := &workflowpb.Function{
		Name:   "greet",
		Params: []string{"name"},
		Code:   &workflowpb.Function_Body{Body: BODY},
	}

	raw, err := protojson.Marshal(fn)
	if err != nil {
		t.Fatal(err)
	}

	text := string(raw)
	if !strings.Contains(text, `"body"`) || !strings.Contains(text, `"params"`) {
		t.Fatalf("want the body and the parameters, got %s", text)
	}

	fn.Code = &workflowpb.Function_Statements{Statements: new(workflowpb.Statements)}

	if fn.GetBody() != "" {
		t.Fatal("want setting the statements to clear the body")
	}
}

// TestCall_ArgsKeepJsonKinds checks literal arguments round-trip as JSON
// values, a string, a number and an object, and that a name travels as an
// operand of its own.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
//   - 2026-09-30 00:44: an argument is a value or a parameter, each under a key
//     of its own
//   - 2026-10-02 01:46: args are values, and a name is an operand
func TestCall_ArgsKeepJsonKinds(t *testing.T) {
	list, err := structpb.NewList([]any{
		"alice",
		3,
		map[string]any{"k": 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := protojson.Marshal(&workflowpb.Call{Function: "greet", Args: list.GetValues()})
	if err != nil {
		t.Fatal(err)
	}

	var site map[string]any

	err = json.Unmarshal(raw, &site)
	if err != nil {
		t.Fatal(err)
	}

	args, ok := site["args"].([]any)
	if !ok || len(args) != len(list.GetValues()) {
		t.Fatalf("want %d args, got %s", len(list.GetValues()), raw)
	}

	if args[0] != "alice" {
		t.Fatalf("want a string, got %#v", args[0])
	}

	if args[1] != float64(3) {
		t.Fatalf("want the number 3, got %#v", args[1])
	}

	obj, ok := args[2].(map[string]any)
	if !ok || obj["k"] != float64(1) {
		t.Fatalf("want an object, got %#v", args[2])
	}

	named := &workflowpb.Operand{Source: &workflowpb.Operand_Name{Name: "word"}}

	raw, err = protojson.Marshal(named)
	if err != nil {
		t.Fatal(err)
	}

	if string(raw) != `{"name":"word"}` {
		t.Fatalf("want the name under name, got %s", raw)
	}
}

// TestFlow_DecodesWhatAUserInterfaceSends checks the authored JSON decodes into
// a flow whose main is statements, holding a spawn bound to a name and a join
// of that name.
//
// Revisions:
//   - 2026-10-02 01:46: initial creation
func TestFlow_DecodesWhatAUserInterfaceSends(t *testing.T) {
	flow := new(workflowpb.Flow)

	err := protojson.Unmarshal([]byte(AUTHORED), flow)
	if err != nil {
		t.Fatal(err)
	}

	if flow.GetFunctions()[0].GetBody() != BODY {
		t.Fatalf("want greet as text, got %v", flow.GetFunctions()[0])
	}

	called := flow.GetFunctions()[1].GetStatements().GetStatement()[0].GetCall()
	if called.GetFunction() != "greet" || called.GetArgs()[0].GetStringValue() != "west" {
		t.Fatalf("want task to call greet with west, got %v", called)
	}

	spawned := flow.GetMain().GetStatement()[0].GetSpawn()
	if spawned.GetBinding() != "h" || spawned.GetCall().GetFunction() != "task" {
		t.Fatalf("want main to spawn task as h, got %v", spawned)
	}

	if flow.GetText() != "" {
		t.Fatal("want main as statements, not as text")
	}
}

// TestGraph_IsACallGraph checks a graph reads and writes as a user interface
// draws it: functions by name with their status, calls as caller and callee,
// and a cause naming a function.
//
// Revisions:
//   - 2026-10-02 01:46: initial creation, as TestGraph_ReportsWhatARunDid
//   - 2026-10-02 15:34: a call graph, of functions and the calls between them
func TestGraph_IsACallGraph(t *testing.T) {
	held := new(workflowpb.Graph)

	err := protojson.Unmarshal([]byte(GRAPH), held)
	if err != nil {
		t.Fatal(err)
	}

	if held.GetCalls()[0].GetCallee() != "boom" {
		t.Fatalf("want main's first call to be boom, got %v", held.GetCalls())
	}

	_Same(t, held, GRAPH)
}

// TestChange_IsAJSONPatch checks a change reads and writes as RFC 6902 JSON:
// op spelled as the RFC spells it, path and from as pointers, value as any
// JSON, and nothing written for a field an operation does not take.
//
// Revisions:
//   - 2026-10-02 15:21: initial creation
//   - 2026-10-02 15:34: compares through _Same, which the graph shares
func TestChange_IsAJSONPatch(t *testing.T) {
	held := new(workflowpb.Change)

	err := protojson.Unmarshal([]byte(CHANGE), held)
	if err != nil {
		t.Fatal(err)
	}

	if held.GetOperations()[0].GetOp() != "add" {
		t.Fatalf("want the first operation an add, got %v", held.GetOperations()[0])
	}

	_Same(t, held, CHANGE)
}

// _Same fails the test unless message writes as the JSON want states, whatever
// the spacing or the order of its members.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation, from TestChange_IsAJSONPatch's body
func _Same(t *testing.T, message proto.Message, want string) {
	t.Helper()

	raw, err := protojson.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}

	var expected, got any

	err = json.Unmarshal([]byte(want), &expected)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(raw, &got)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("got %s, want %s", raw, want)
	}
}
