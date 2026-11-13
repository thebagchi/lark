package tests

import (
	"errors"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestCheck_RefusesANameThatIsNotAnIdentifier proves Check refuses each name
// field the generated script writes as an identifier, when the name is not a
// Starlark identifier or is a keyword (Q300): "Check is what a flow must
// satisfy before anything generates from it". A parameter named "000" was
// the fuzz crasher: Check accepted it and Emit wrote def greet(000):.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
func TestCheck_RefusesANameThatIsNotAnIdentifier(t *testing.T) {
	t.Parallel()

	cases := []_Refusal{
		{
			name: "a parameter of digits",
			flow: `{"functions": [{"name": "greet", "params": ["000"],
				"body": "pass"}],
				"text": "pass"}`,
			err: graph.ErrForm,
		},
		{
			name: "a parameter that is a keyword",
			flow: `{"functions": [{"name": "greet", "params": ["lambda"],
				"body": "pass"}],
				"text": "pass"}`,
			err: graph.ErrForm,
		},
		{
			name: "a parameter that is reserved",
			flow: `{"functions": [{"name": "greet", "params": ["class"],
				"body": "pass"}],
				"text": "pass"}`,
			err: graph.ErrForm,
		},
		{
			name: "a parameter with a space",
			flow: `{"functions": [{"name": "greet", "params": ["a b"],
				"body": "pass"}],
				"text": "pass"}`,
			err: graph.ErrForm,
		},
		{
			name: "an empty parameter",
			flow: `{"functions": [{"name": "greet", "params": [""], "body": "pass"}],
				"text": "pass"}`,
			err: graph.ErrForm,
		},
		{
			name: "a function with a dash",
			flow: `{"functions": [{"name": "say-hi",
				"body": "pass"}], "text": "pass"}`,
			err: graph.ErrForm,
		},
		{
			name: "a function that is a keyword",
			flow: `{"functions": [{"name": "pass", "body": "pass"}], "text": "pass"}`,
			err:  graph.ErrForm,
		},
		{
			name: "a function with no name",
			flow: `{"functions": [{"body": "pass"}], "text": "pass"}`,
			err:  graph.ErrForm,
		},
		{
			name: "a call of a path with an empty part",
			flow: `{"main": {"statement": [{"call": {"function": "a..b",
				"builtin": true}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a call of a path with a keyword part",
			flow: `{"main": {"statement": [{"call": {"function": "a.pass",
				"builtin": true}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a call of a name with a dash",
			flow: `{"main": {"statement": [{"call": {"function": "a-b",
				"builtin": true}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a call of a keyword",
			flow: `{"main": {"statement": [{"call": {"function": "for",
				"builtin": true}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a result that starts with a digit",
			flow: `{"functions": [{"name": "f", "body": "pass"}],
				"main": {"statement": [{"call": {"function": "f",
				"result": "1st"}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a keyword argument that is a keyword",
			flow: `{"functions": [{"name": "f", "params": ["x"], "body": "pass"}],
				"main": {"statement": [{"call": {"function": "f",
					"keywords": [{"name": "if",
				"value": {"literal": {"int": "1"}}}]}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "an operand with a dash",
			flow: `{"functions": [{"name": "f", "params": ["x"], "body": "pass"}],
				"constants": {"a-b": {"value": {"int": "1"}}},
				"main": {"statement": [{"call": {"function": "f",
					"operands": [{"name": "a-b"}]}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a constant with a space",
			flow: `{"constants": {"a b": {"value": {"int": "1"}}}, "text": "pass"}`,
			err:  graph.ErrForm,
		},
		{
			name: "an argument with a dot",
			flow: `{"args": {"a.b": {"name": "a.b"}}, "text": "pass"}`,
			err:  graph.ErrForm,
		},
		{
			name: "a handle of digits in a spawn",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [
					{"spawn": {"binding": "000", "call": {"function": "work"}}}
				]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a handle that is a keyword in a join",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [
					{"spawn": {"binding": "h", "call": {"function": "work"}}},
					{"join": {"bindings": ["in"]}}
				]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a join result with a space",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [
					{"spawn": {"binding": "h", "call": {"function": "work"}}},
					{"join": {"bindings": ["h"], "result": "a b"}}
				]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a handle with a dash in a cancel",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [
					{"spawn": {"binding": "h", "call": {"function": "work"}}},
					{"cancel": {"bindings": ["h-1"]}}
				]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a condition that is not a name",
			flow: `{"constants": {"a": {"value": {"bool": true}}},
				"functions": [{"name": "f", "body": "pass"}],
				"main": {"statement": [{"if": {"condition": {"name": "a b"},
					"then": {"call": {"function": "f"}}}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a match expression that is not a name",
			flow: `{"functions": [{"name": "f", "body": "pass"}],
				"main": {"statement": [{"match": {"expression": {"name": "1x"},
					"default": {"call": {"function": "f"}}}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a loop count that is not a name",
			flow: `{"functions": [{"name": "f", "body": "pass"}],
				"main": {"statement": [{"loop": {"times": {"name": "n n"},
					"body": {"call": {"function": "f"}}}}]}}`,
			err: graph.ErrForm,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()

				err := graph.Check(_Flow(
					t,
					tc.flow,
				))
				if !errors.Is(
					err,
					tc.err,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tc.err,
					)
				}
			},
		)
	}
}

// TestCheck_AcceptsNamesThatAreIdentifiers proves the check does not refuse a
// name Starlark reads as an identifier: a leading underscore, a letter outside
// ASCII, the predeclared names None and len, which a script may shadow, and a
// path such as hex.to_bytes, which a flow uses to call a member of a predeclared
// name.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
func TestCheck_AcceptsNamesThatAreIdentifiers(t *testing.T) {
	t.Parallel()

	err := graph.Check(_Flow(
		t,
		`{"functions": [{"name": "_say", "params": ["é", "len"], "body": "pass"}],
			"constants": {"None": {"value": {"int": "1"}}},
			"main": {"statement": [
				{"call": {"function": "_say",
					"operands": [{"name": "None"}, {"name": "None"}],
				"result": "_r1"}},
				{"call": {"function": "hex.to_bytes", "builtin": true}}
			]}}`,
	))
	if err != nil {
		t.Fatalf(
			"got %v, want none",
			err,
		)
	}
}

// TestEmit_AFlowCheckAcceptsDerivesBack proves the flow of the fuzz crasher
// (graph-F2) is refused, and a flow with the same shape and identifier names
// is written as a script that derives.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
func TestEmit_AFlowCheckAcceptsDerivesBack(t *testing.T) {
	t.Parallel()

	for name, ok := range map[string]bool{
		"000":  false,
		"name": true,
	} {
		var (
			flow = _Flow(
				t,
				`{"functions": [{"name": "greet", "params": ["`+name+`"],
					"body": "pass"}],
					"main": {"statement": [
						{"call": {"function": "greet",
							"args": [{"int": "1"}]}}]}}`,
			)
			err = graph.Check(flow)
		)
		if ok != (err == nil) {
			t.Fatalf(
				"%q: check got %v, want ok %v",
				name,
				err,
				ok,
			)
		}

		if !ok {
			continue
		}

		out, err := graph.Emit(flow)
		if err != nil {
			t.Fatalf(
				"emit: %v",
				err,
			)
		}

		_Derived(
			t,
			string(out),
		)
	}
}
