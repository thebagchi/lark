package tests

import (
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

const (
	FLOW_FILL = 200
	FLOW_MAX  = 700
)

// _FlowSite is one print site that a name from a flow file can reach. Its flow
// holds @N, and @M where a second name is printed; each stands for a JSON
// string that starts with a line break and is longer than the quote limit.
//
// Revisions:
//   - 2026-11-06 06:22: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _FlowSite struct {
	site string
	emit bool
	flow string
}

// TestRefusal_FlowNamesAreQuoted proves each print site of graph that can show
// a name from a flow file writes it quoted and bounded (Q4, Q208, B128). A JSON
// string can hold a raw line break, so a name printed as it came would forge a
// line of the log (CWE-117). One row per site: dropping the quote at that site
// makes its row fail.
//
// Revisions:
//   - 2026-11-06 00:18: initial creation (C63, B128, Q208)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
//   - 2026-11-13 23:55: the case is named for the argument count, formerly arity (C136, Q475)
func TestRefusal_FlowNamesAreQuoted(t *testing.T) {
	var (
		first = "\"\\nN" + strings.Repeat(
			"x",
			FLOW_FILL,
		) + "\""
		second = "\"\\nM" + strings.Repeat(
			"y",
			FLOW_FILL,
		) + "\""
		sites = []_FlowSite{
			{
				"check _Bodies: function with no code",
				false,
				`{"functions": [{"name": @N}], "text": "pass"}`,
			},
			{
				"check _Fits: both lists",
				false,
				`{"functions": [{"name": @N, "params": ["a"], ` +
					`"body": "pass"}], ` +
					`"main": {"statement": [{"call": {"function": ` +
					`@N, "args": [{"int": "1"}], ` +
					`"operands": [{"name": @N}]}}]}}`,
			},
			{
				"check _Fits: builtin declared by the flow",
				false,
				`{"functions": [{"name": @N, "body": "pass"}], ` +
					`"main": {"statement": [{"call": {"function": @N, ` +
					`"builtin": true}}]}}`,
			},
			{
				"check _Fits: unresolved call",
				false,
				`{"main": {"statement": [{"call": {"function": @N}}]}}`,
			},
			{
				"check _Fits: argument count",
				false,
				`{"functions": [{"name": @N, "params": ["a", "b"], ` +
					`"body": "pass"}], ` +
					`"main": {"statement": [{"call": {"function": @N, ` +
					`"args": [{"string": "a"}]}}]}}`,
			},
			{
				"check _Phrased: keyword without a name",
				false,
				`{"functions": [{"name": @N, "body": "pass"}], ` +
					`"main": {"statement": [{"call": {"function": @N, ` +
					`"keywords": [{"name": "", ` +
					`"value": {"literal": {"int": "1"}}}]}}]}}`,
			},
			{
				"check _Phrased: expression that is not one",
				false,
				`{"functions": [{"name": @N, "params": ["a"], ` +
					`"body": "pass"}], ` +
					`"main": {"statement": [{"call": {"function": @N, ` +
					`"operands": [{"expression": "a; b"}]}}]}}`,
			},
			{
				"check _Computed: constant names nothing",
				false,
				`{"functions": [{"name": "f", "params": ["x"], ` +
					`"body": "pass"}], ` +
					`"constants": {@N: {"call": {"function": "f", ` +
					`"operands": [{"name": @M}]}}}, "text": "pass"}`,
			},
			{
				"check _Resolved: function with an unresolved call",
				false,
				`{"functions": [{"name": @N, ` +
					`"statements": {"statement": [{"join": ` +
					`{"bindings": ["z"]}}]}}], ` +
					`"main": {"statement": [{"call": {"function": @N}}]}}`,
			},
			{
				"distinct: two functions",
				false,
				`{"functions": [{"name": @N, "body": "pass"}, {"name": @N, ` +
					`"body": "pass"}], "text": "pass"}`,
			},
			{
				"distinct: function and constant",
				false,
				`{"functions": [{"name": @N, "body": "pass"}], ` +
					`"constants": {@N: {"value": {"int": "1"}}}, ` +
					`"text": "pass"}`,
			},
			{
				"distinct: constant and argument",
				false,
				`{"constants": {@N: {"value": {"int": "1"}}}, ` +
					`"args": {@N: {"name": @N}}, ` +
					`"text": "pass"}`,
			},
			{
				"visible _Spawn: two spawns of one binding",
				false,
				`{"functions": [{"name": "work", "body": "pass"}], ` +
					`"main": {"statement": [{"spawn": {"binding": @N, ` +
					`"call": {"function": "work"}}}, ` +
					`{"spawn": {"binding": @N, ` +
					`"call": {"function": "work"}}}]}}`,
			},
			{
				"visible _Names: join of an unspawned binding",
				false,
				`{"main": {"statement": [{"join": {"bindings": [@N]}}]}}`,
			},
			{
				"visible _Operands: operand naming nothing",
				false,
				`{"functions": [{"name": @N, "params": ["x"], ` +
					`"body": "pass"}], ` +
					`"main": {"statement": [{"call": {"function": @N, ` +
					`"operands": [{"name": @M}]}}]}}`,
			},
			{
				"visible _Name: condition naming nothing",
				false,
				`{"functions": [{"name": "f", "body": "pass"}], ` +
					`"main": {"statement": [{"if": ` +
					`{"condition": {"name": @N}, ` +
					`"then": {"call": {"function": "f"}}}}]}}`,
			},
			{
				"statement_write _Code: function with no code",
				true,
				`{"functions": [{"name": @N}], "text": "pass"}`,
			},
			{
				"statement_write _ListBody: empty statement list",
				true,
				`{"functions": [{"name": @N, ` +
					`"statements": {"statement": []}}], ` +
					`"main": {"statement": [{"call": {"function": @N}}]}}`,
			},
			{
				"suite _Parsed: body of only comments",
				true,
				`{"functions": [{"name": @N, "body": "# only"}], ` +
					`"text": "pass"}`,
			},
			{
				"call_write _CallExpr: both lists",
				true,
				`{"functions": [{"name": @N, "body": "pass"}], ` +
					`"main": {"statement": [{"call": {"function": ` +
					`@N, "args": [{"int": "1"}], ` +
					`"operands": [{"name": "x"}]}}]}}`,
			},
			{
				"call_write _Target: spawn call with a result",
				true,
				`{"functions": [{"name": @N, "body": "pass"}], ` +
					`"main": {"statement": [{"spawn": ` +
					`{"call": {"function": @N, ` +
					`"result": "r"}}}]}}`,
			},
			{
				"branch_write _Condition: call with a result",
				true,
				`{"functions": [{"name": @N, "body": "pass"}], ` +
					`"main": {"statement": [{"if": {"condition": ` +
					`{"call": {"function": @N, ` +
					`"result": "r"}}, "then": ` +
					`{"call": {"function": @N}}}}]}}`,
			},
			{
				"branch_write _Expression: call with a result",
				true,
				`{"functions": [{"name": @N, "body": "pass"}], ` +
					`"main": {"statement": [{"match": {"expression": ` +
					`{"call": {"function": @N, ` +
					`"result": "r"}}, "cases": [{"value": "a", ` +
					`"statement": {"call": {"function": @N}}}]}}]}}`,
			},
			{
				"constant _Bound: call with a result",
				true,
				`{"functions": [{"name": "f", "body": "pass"}], ` +
					`"constants": {@N: {"call": {"function": "f", ` +
					`"result": "r"}}}, "text": "pass"}`,
			},
			{
				"constant _Bound: both lists",
				true,
				`{"functions": [{"name": "f", "body": "pass"}], ` +
					`"constants": {@N: {"call": {"function": ` +
					`"f", "args": [{"int": "1"}], ` +
					`"operands": [{"name": "x"}]}}}, "text": "pass"}`,
			},
			{
				"constant _Bound: no value",
				true,
				`{"functions": [{"name": "f", "body": "pass"}], ` +
					`"constants": {@N: {}}, "text": "pass"}`,
			},
		}
	)

	for _, site := range sites {
		t.Run(
			site.site,
			func(t *testing.T) {
				text := strings.NewReplacer(
					"@N",
					first,
					"@M",
					second,
				).Replace(site.flow)

				var err error
				if site.emit {
					_, err = graph.Emit(_Flow(
						t,
						text,
					))
				} else {
					err = graph.Check(_Flow(
						t,
						text,
					))
				}

				_Quoted(
					t,
					err,
				)
			},
		)
	}
}

// _Quoted fails the test unless err is one line, at most FLOW_MAX bytes, with
// the line break of the name written as an escape.
//
// Revisions:
//   - 2026-11-06 00:18: initial creation (C63, B128, Q208)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Quoted(
	t *testing.T,
	err error,
) {
	t.Helper()

	if err == nil {
		t.Fatal("got nil, want a refusal")
	}

	got := err.Error()
	if strings.Contains(
		got,
		"\n",
	) || !strings.Contains(
		got,
		"\\n",
	) {
		t.Fatalf(
			"name not quoted in %q",
			got[:min(
				len(got),
				200,
			)],
		)
	}

	if len(got) > FLOW_MAX {
		t.Fatalf(
			"error has %d bytes, want at most %d",
			len(got),
			FLOW_MAX,
		)
	}
}
