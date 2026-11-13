package tests

import (
	"errors"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// TestCheck_Fixtures checks that each fixture's flow is one Check accepts.
//
// Revisions:
//   - 2026-10-02 00:18: initial creation
//   - 2026-11-09 11:25: reads FIXTURES, not testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_Fixtures(t *testing.T) {
	t.Parallel()

	for _, fixture := range FIXTURES {
		t.Run(
			fixture.name,
			func(t *testing.T) {
				t.Parallel()

				err := graph.Check(_Flow(
					t,
					fixture.flow,
				))
				if err != nil {
					t.Fatalf(
						"check: %v",
						err,
					)
				}
			},
		)
	}
}

// TestCheck_Refuses is the flow each sentinel rejects.
//
// Revisions:
//   - 2026-10-02 00:18: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCarry and ErrFork, formerly ErrNotCarried and ErrNotForked (C121,
//     Q348)
//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
func TestCheck_Refuses(t *testing.T) {
	t.Parallel()

	cases := []_Refusal{
		{
			name: "two functions of one name",
			flow: `{"functions": [{"name": "f", "body": "pass"}, {"name": "f", ` +
				`"body": "pass"}],
				"text": "pass"}`,
			err: graph.ErrDuplicate,
		},
		{
			name: "a function named main",
			flow: `{"functions": [{"name": "main", "body": "pass"}], "text": "pass"}`,
			err:  graph.ErrDuplicate,
		},
		{
			name: "a function and a constant of one name",
			flow: `{"functions": [{"name": "f", "body": "pass"}],
				"constants": {"f": {"value": {"int": "1"}}}, "text": "pass"}`,
			err: graph.ErrDuplicate,
		},
		{
			name: "a constant and an argument of one name",
			flow: `{"constants": {"x": {"value": {"int": "1"}}},
				"args": {"x": {"name": "x"}},
				"text": "pass"}`,
			err: graph.ErrDuplicate,
		},
		{
			name: "no main",
			flow: `{}`,
			err:  graph.ErrForm,
		},
		{
			name: "an empty main",
			flow: `{"main": {"statement": []}}`,
			err:  graph.ErrForm,
		},
		{
			name: "a body of only comments",
			flow: `{"functions": [{"name": "f", "body": "# only"}], "text": "pass"}`,
			err:  graph.ErrForm,
		},
		{
			name: "a function with no code",
			flow: `{"functions": [{"name": "f"}], "text": "pass"}`,
			err:  graph.ErrForm,
		},
		{
			name: "too few arguments",
			flow: `{"functions": [{"name": "sign", "params": ["text", "mark"], ` +
				`"body": "pass"}],
				"main": {"statement": [{"call": {
					"function": "sign", "args": [{"string": "a"}]}}]}}`,
			err: spelling.ErrArity,
		},
		{
			name: "too many arguments",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [{"spawn": {"call": {
					"function": "work", "args": [{"int": "1"}]}}}]}}`,
			err: spelling.ErrArity,
		},
		{
			name: "both lists",
			flow: `{"functions": [{"name": "f", "params": ["a", "b"], "body": "pass"}],
				"main": {"statement": [{"call": {"function": "f",
					"args": [{"int": "1"}],
					"operands": [{"name": "f"}]}}]}}`,
			err: graph.ErrForm,
		},
		{
			name: "a call of a function the flow does not declare",
			flow: `{"main": {"statement": [{"call": {"function": "nowhere"}}]}}`,
			err:  graph.ErrUnresolved,
		},
		{
			name: "an operand naming nothing",
			flow: `{"functions": [{"name": "f", "params": ["x"], "body": "pass"}],
				"main": {"statement": [{"call": {"function": "f",
					"operands": [{"name": "missing"}]}}]}}`,
			err: graph.ErrUnresolved,
		},
		{
			name: "a condition naming nothing",
			flow: `{"functions": [{"name": "f", "body": "pass"}],
				"main": {"statement": [{"if": {"condition": {"name": "missing"},
					"then": {"call": {"function": "f"}}}}]}}`,
			err: graph.ErrUnresolved,
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
			err: graph.ErrUnresolved,
		},
		{
			name: "a constant naming an argument",
			flow: `{"functions": [{"name": "f", "params": ["x"], "body": "pass"}],
				"constants": {"label": {"call": {"function": "f",
					"operands": [{"name": "port"}]}}},
				"args": {"port": {"name": "port"}}, "text": "pass"}`,
			err: graph.ErrUnresolved,
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
			err: graph.ErrCarry,
		},
		{
			name: "a join of a binding no spawn holds",
			flow: `{"main": {"statement": [{"join": {"bindings": ["h"]}}]}}`,
			err:  graph.ErrFork,
		},
		{
			name: "a join of a binding in the other branch",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [{"if": {"condition": {"value": true},
					"then": {"spawn": {
						"binding": "h", "call": {"function": "work"}}},
					"else": {"join": {"bindings": ["h"]}}}}]}}`,
			err: graph.ErrFork,
		},
		{
			name: "two spawns of one binding",
			flow: `{"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [
					{"spawn": {"binding": "h", "call": {"function": "work"}}},
					{"spawn": {"binding": "h", "call": {"function": "work"}}}
				]}}`,
			err: graph.ErrFork,
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
