// Package workflow_test exercises the workflow schema's JSON: a flow as a user
// interface authors it, a graph as a run reports it, and a change to that
// graph as a JSON Patch.
package tests

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

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
	//
	// Revisions:
	//   - 2026-11-13 10:16: the args of the flow are tagged values (C132, Q422)
	AUTHORED = `{
		"functions": [
			{"name": "greet", "params": ["name"], "body": "return \"hello \" + name"},
			{"name": "task", "statements": {"statement": [
				{"call": {"function": "greet", "args": [{"string": "west"}]}}
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFunction_CarriesBodyOrStatements(t *testing.T) {
	var fn = &workflowpb.Function{
		Name:   "greet",
		Params: []string{"name"},
		Code: &workflowpb.Function_Body{
			Body: BODY,
		},
	}

	raw, err := protojson.Marshal(fn)
	if err != nil {
		t.Fatal(err)
	}

	text := string(raw)
	if !strings.Contains(
		text,
		`"body"`,
	) || !strings.Contains(
		text,
		`"params"`,
	) {
		t.Fatalf(
			"want the body and the parameters, got %s",
			text,
		)
	}

	fn.Code = &workflowpb.Function_Statements{
		Statements: new(workflowpb.Statements),
	}

	if fn.GetBody() != "" {
		t.Fatal("want setting the statements to clear the body")
	}
}

// TestCall_ArgsKeepJsonKinds checks literal arguments round-trip as tagged
// values, a string, an int and a map, each an object named for its kind, and
// that a name travels as an operand of its own.
//
// Revisions:
//   - 2026-09-20 18:40: initial creation
//   - 2026-09-30 00:44: an argument is a value or a parameter, each under a key
//     of its own
//   - 2026-10-02 01:46: args are values, and a name is an operand
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-13 10:15: the args are workflow.Value, tagged by kind (C132, Q407, Q422)
func TestCall_ArgsKeepJsonKinds(t *testing.T) {
	var args = []*workflowpb.Value{
		{
			Kind: &workflowpb.Value_String_{
				String_: "alice",
			},
		},
		{
			Kind: &workflowpb.Value_Int{
				Int: 3,
			},
		},
		{
			Kind: &workflowpb.Value_Map{
				Map: &workflowpb.Map{
					Entries: map[string]*workflowpb.Value{
						"k": {
							Kind: &workflowpb.Value_Int{
								Int: 1,
							},
						},
					},
				},
			},
		},
	}

	raw, err := protojson.Marshal(&workflowpb.Call{
		Function: "greet",
		Args:     args,
	})
	if err != nil {
		t.Fatal(err)
	}

	var site map[string]any

	err = json.Unmarshal(
		raw,
		&site,
	)
	if err != nil {
		t.Fatal(err)
	}

	var want = []any{
		map[string]any{"string": "alice"},
		map[string]any{"int": "3"},
		map[string]any{"map": map[string]any{
			"entries": map[string]any{
				"k": map[string]any{"int": "1"},
			},
		}},
	}

	if !reflect.DeepEqual(
		site["args"],
		want,
	) {
		t.Fatalf(
			"want the args %v tagged by kind, got %s",
			want,
			raw,
		)
	}

	named := &workflowpb.Operand{
		Source: &workflowpb.Operand_Name{
			Name: "word",
		},
	}

	raw, err = protojson.Marshal(named)
	if err != nil {
		t.Fatal(err)
	}

	if string(raw) != `{"name":"word"}` {
		t.Fatalf(
			"want the name under name, got %s",
			raw,
		)
	}
}

// TestFlow_DecodesWhatAUserInterfaceSends checks the authored JSON decodes into
// a flow whose main is statements, holding a spawn bound to a name and a join
// of that name.
//
// Revisions:
//   - 2026-10-02 01:46: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-13 10:16: reads the string kind of an arg (C132, Q422)
func TestFlow_DecodesWhatAUserInterfaceSends(t *testing.T) {
	var (
		flow = new(workflowpb.Flow)
		err  = protojson.Unmarshal(
			[]byte(AUTHORED),
			flow,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	if flow.GetFunctions()[0].GetBody() != BODY {
		t.Fatalf(
			"want greet as text, got %v",
			flow.GetFunctions()[0],
		)
	}

	called := flow.GetFunctions()[1].GetStatements().GetStatement()[0].GetCall()
	if called.GetFunction() != "greet" || called.GetArgs()[0].GetString_() != "west" {
		t.Fatalf(
			"want task to call greet with west, got %v",
			called,
		)
	}

	spawned := flow.GetMain().GetStatement()[0].GetSpawn()
	if spawned.GetBinding() != "h" || spawned.GetCall().GetFunction() != "task" {
		t.Fatalf(
			"want main to spawn task as h, got %v",
			spawned,
		)
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGraph_IsACallGraph(t *testing.T) {
	var (
		held = new(workflowpb.Graph)
		err  = protojson.Unmarshal(
			[]byte(GRAPH),
			held,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	if held.GetCalls()[0].GetCallee() != "boom" {
		t.Fatalf(
			"want main's first call to be boom, got %v",
			held.GetCalls(),
		)
	}

	_Same(
		t,
		held,
		GRAPH,
	)
}

// TestChange_IsAJSONPatch checks a change reads and writes as RFC 6902 JSON:
// op spelled as the RFC spells it, path and from as pointers, value as any
// JSON, and nothing written for a field an operation does not take.
//
// Revisions:
//   - 2026-10-02 15:21: initial creation
//   - 2026-10-02 15:34: compares through _Same, which the graph shares
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestChange_IsAJSONPatch(t *testing.T) {
	var (
		held = new(workflowpb.Change)
		err  = protojson.Unmarshal(
			[]byte(CHANGE),
			held,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	if held.GetOperations()[0].GetOp() != "add" {
		t.Fatalf(
			"want the first operation an add, got %v",
			held.GetOperations()[0],
		)
	}

	_Same(
		t,
		held,
		CHANGE,
	)
}

// _Same fails the test unless message writes as the JSON want states, whatever
// the spacing or the order of its members.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation, from TestChange_IsAJSONPatch's body
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Same(
	t *testing.T,
	message proto.Message,
	want string,
) {
	t.Helper()

	raw, err := protojson.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}

	var expected, got any

	err = json.Unmarshal(
		[]byte(want),
		&expected,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(
		raw,
		&got,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		got,
		expected,
	) {
		t.Fatalf(
			"got %s, want %s",
			raw,
			want,
		)
	}
}

// TestStatus_NotStartedIsAdditive checks the status a run gives a function it
// meant to start and never did. The values older hosts read keep their
// numbers, and the new one is next after them.
//
// Success: each status has its number; NOT_STARTED is 6 and reads from and
// writes to JSON by name; a graph holding it round-trips.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStatus_NotStartedIsAdditive(t *testing.T) {
	numbers := map[workflowpb.Status]int32{
		workflowpb.Status_STATUS_UNSPECIFIED: 0,
		workflowpb.Status_STATUS_PENDING:     1,
		workflowpb.Status_STATUS_RUNNING:     2,
		workflowpb.Status_STATUS_SUCCEEDED:   3,
		workflowpb.Status_STATUS_FAILED:      4,
		workflowpb.Status_STATUS_CANCELLED:   5,
		workflowpb.Status_STATUS_NOT_STARTED: 6,
	}

	for status, number := range numbers {
		if int32(status) != number {
			t.Fatalf(
				"%v is %d, want %d",
				status,
				int32(status),
				number,
			)
		}
	}

	var (
		text = `{"status":"STATUS_FAILED",` +
			`"functions":[{"name":"patient","status":"STATUS_NOT_STARTED"}]}`
		graph = new(workflowpb.Graph)
	)

	err := protojson.Unmarshal(
		[]byte(text),
		graph,
	)
	if err != nil {
		t.Fatalf(
			"unmarshal: %v",
			err,
		)
	}

	if graph.GetFunctions()[0].GetStatus() != workflowpb.Status_STATUS_NOT_STARTED {
		t.Fatalf(
			"status %v, want NOT_STARTED",
			graph.GetFunctions()[0].GetStatus(),
		)
	}

	raw, err := protojson.Marshal(graph)
	if err != nil {
		t.Fatalf(
			"marshal: %v",
			err,
		)
	}

	if !strings.Contains(
		string(raw),
		"STATUS_NOT_STARTED",
	) {
		t.Fatalf(
			"json %s lacks the name",
			raw,
		)
	}
}
