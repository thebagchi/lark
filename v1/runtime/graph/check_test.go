package graph_test

import (
	"errors"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/graph"
)

// TestCheck_Fixtures checks that each fixture's flow is one Check accepts.
//
// Revisions:
//   - 2026-10-02 00:18: initial creation
func TestCheck_Fixtures(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"dag", "every", "shapes"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := graph.Check(_Flow(t, _Read(t, "testdata/"+name+".json")))
			if err != nil {
				t.Fatalf("check: %v", err)
			}
		})
	}
}

// TestCheck_Refuses is the flow each sentinel rejects.
//
// Revisions:
//   - 2026-10-02 00:18: initial creation
func TestCheck_Refuses(t *testing.T) {
	t.Parallel()

	cases := []_Refusal{
		{
			name: "two functions of one name",
			flow: `{"functions": [{"name": "f", "body": "pass"}, {"name": "f", ` +
				`"body": "pass"}],
				"text": "pass"}`,
			err: graph.ERR_DUPLICATE,
		},
		{
			name: "a function named main",
			flow: `{"functions": [{"name": "main", "body": "pass"}], "text": "pass"}`,
			err:  graph.ERR_DUPLICATE,
		},
		{
			name: "a function and a constant of one name",
			flow: `{"functions": [{"name": "f", "body": "pass"}],
				"constants": {"f": {"value": 1}}, "text": "pass"}`,
			err: graph.ERR_DUPLICATE,
		},
		{
			name: "a constant and an argument of one name",
			flow: `{"constants": {"x": {"value": 1}}, "args": {"x": {"name": "x"}},
				"text": "pass"}`,
			err: graph.ERR_DUPLICATE,
		},
		{name: "no main", flow: `{}`, err: graph.ERR_NO_BODY},
		{
			name: "an empty main",
			flow: `{"main": {"statement": []}}`,
			err:  graph.ERR_NO_BODY,
		},
		{
			name: "a body of only comments",
			flow: `{"functions": [{"name": "f", "body": "# only"}], "text": "pass"}`,
			err:  graph.ERR_NO_BODY,
		},
		{
			name: "a function with no code",
			flow: `{"functions": [{"name": "f"}], "text": "pass"}`,
			err:  graph.ERR_NO_BODY,
		},
		{
			name: "too few arguments",
			flow: `{"functions": [{"name": "sign", "params": ["text", "mark"], ` +
				`"body": "pass"}],
				"main": {"statement": [{"call": {
					"function": "sign", "args": ["a"]}}]}}`,
			err: graph.ERR_ARITY,
		},
		{
			name: "too many arguments",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [{"spawn": {"call": {
					"function": "work", "args": [1]}}}]}}`,
			err: graph.ERR_ARITY,
		},
		{
			name: "both lists",
			flow: `{"functions": [{"name": "f", "params": ["a", "b"], "body": "pass"}],
				"main": {"statement": [{"call": {"function": "f", "args": [1],
					"operands": [{"name": "f"}]}}]}}`,
			err: graph.ERR_ARGUMENTS,
		},
		{
			name: "a call of a function the flow does not declare",
			flow: `{"main": {"statement": [{"call": {"function": "nowhere"}}]}}`,
			err:  graph.ERR_UNRESOLVED,
		},
		{
			name: "an operand naming nothing",
			flow: `{"functions": [{"name": "f", "params": ["x"], "body": "pass"}],
				"main": {"statement": [{"call": {"function": "f",
					"operands": [{"name": "missing"}]}}]}}`,
			err: graph.ERR_UNRESOLVED,
		},
		{
			name: "a condition naming nothing",
			flow: `{"functions": [{"name": "f", "body": "pass"}],
				"main": {"statement": [{"if": {"condition": {"name": "missing"},
					"then": {"call": {"function": "f"}}}}]}}`,
			err: graph.ERR_UNRESOLVED,
		},
		{
			name: "a result used after the branch that bound it",
			flow: `{"functions": [{"name": "once", "body": "pass"},
					{"name": "sign", "params": ["text"], "body": "pass"}],
				"main": {"statement": [
					{"if": {"condition": {"value": true},
						"then": {"call": {
							"function": "once", "result": "got"}}}},
					{"call": {
						"function": "sign", "operands": [{"name": "got"}]}}
				]}}`,
			err: graph.ERR_UNRESOLVED,
		},
		{
			name: "a constant naming an argument",
			flow: `{"functions": [{"name": "f", "params": ["x"], "body": "pass"}],
				"constants": {"label": {"call": {"function": "f",
					"operands": [{"name": "port"}]}}},
				"args": {"port": {"name": "port"}}, "text": "pass"}`,
			err: graph.ERR_UNRESOLVED,
		},
		{
			name: "constants naming each other",
			flow: `{"functions": [{"name": "f", "params": ["x"], "body": "pass"}],
				"constants": {
					"a": {"call": {
						"function": "f", "operands": [{"name": "b"}]}},
					"b": {"call": {
						"function": "f", "operands": [{"name": "a"}]}}
				}, "text": "pass"}`,
			err: graph.ERR_CONSTANT,
		},
		{
			name: "a join of a binding no spawn holds",
			flow: `{"main": {"statement": [{"join": {"bindings": ["h"]}}]}}`,
			err:  graph.ERR_NOT_FORKED,
		},
		{
			name: "a join of a binding in the other branch",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [{"if": {"condition": {"value": true},
					"then": {"spawn": {
						"binding": "h", "call": {"function": "work"}}},
					"else": {"join": {"bindings": ["h"]}}}}]}}`,
			err: graph.ERR_NOT_FORKED,
		},
		{
			name: "two spawns of one binding",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [
					{"spawn": {"binding": "h", "call": {"function": "work"}}},
					{"spawn": {"binding": "h", "call": {"function": "work"}}}
				]}}`,
			err: graph.ERR_NOT_FORKED,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := graph.Check(_Flow(t, tc.flow))
			if !errors.Is(err, tc.err) {
				t.Fatalf("got %v, want %v", err, tc.err)
			}
		})
	}
}
