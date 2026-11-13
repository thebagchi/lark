package tests

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
)

// TestCheck_RefusesWhatEmitHasNoFormFor proves Check refuses a constant, an argument, an
// operand and a statement that state nothing or are out of range, which Emit refused with
// "no form" while Check let them through (B296, Q453). Check, Read and Emit give the same
// words, with ErrForm, and the words stay short (Q4).
//
// The check: go test -race -count=1 -run TestCheck_RefusesWhatEmitHasNoFormFor
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 20:18: initial creation (C133, B296, Q453)
func TestCheck_RefusesWhatEmitHasNoFormFor(t *testing.T) {
	for _, tc := range STATES_NOTHING {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var flow = new(workflowpb.Flow)

				err := protojson.Unmarshal(
					[]byte(tc.flow),
					flow,
				)
				if err != nil {
					t.Fatalf(
						"the flow must decode: %v",
						err,
					)
				}

				checked := runtime.Check(flow)
				if checked == nil {
					t.Fatal("Check must refuse the flow")
				}

				if checked.Error() != tc.want {
					t.Fatalf(
						"got %q, want %q",
						checked,
						tc.want,
					)
				}

				if !errors.Is(
					checked,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						checked,
						runtime.ErrForm,
					)
				}

				if len(checked.Error()) > BOUND {
					t.Fatalf(
						"the error is %d bytes, want at most %d",
						len(checked.Error()),
						BOUND,
					)
				}

				_, emitted := runtime.Emit(flow)
				if emitted == nil || emitted.Error() != checked.Error() {
					t.Fatalf(
						"Emit says %v, Check says %v",
						emitted,
						checked,
					)
				}

				_, read := runtime.Read([]byte(tc.flow))
				if read == nil || read.Error() != checked.Error() {
					t.Fatalf(
						"Read says %v, Check says %v",
						read,
						checked,
					)
				}
			},
		)
	}
}

// TestCheck_AcceptsWhatStatesSomething proves the new refusals leave alone an argument
// with no default, an argument whose name is not the name it binds, a constant of a
// value or of a call, and an operand of each source.
//
// The check: go test -race -count=1 -run TestCheck_AcceptsWhatStatesSomething
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 20:18: initial creation (C133, B296, Q453)
func TestCheck_AcceptsWhatStatesSomething(t *testing.T) {
	for _, tc := range STATES_SOMETHING {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := runtime.Read([]byte(tc.flow))
				if err != nil {
					t.Fatal(err)
				}

				_, err = runtime.Emit(flow)
				if err != nil {
					t.Fatalf(
						"Emit refuses a flow Check accepts: %v",
						err,
					)
				}
			},
		)
	}
}

// TestCheck_BoundsTheNameOfAConstantThatStatesNothing proves a name of 5,000 bytes is cut
// in the words of the refusal (Q4).
//
// The check: go test -race -count=1 -run TestCheck_BoundsTheNameOfAConstantThatStatesNothing
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 20:18: initial creation (C133, B296, Q4, Q453)
func TestCheck_BoundsTheNameOfAConstantThatStatesNothing(t *testing.T) {
	_, err := runtime.Read([]byte(
		`{"constants": {"` + strings.Repeat(
			"n",
			5000,
		) + `": {}}, "text": "pass"}`,
	))
	if err == nil || len(err.Error()) > BOUND || !errors.Is(
		err,
		runtime.ErrForm,
	) {
		t.Fatalf(
			"want ErrForm in at most %d bytes, got %v",
			BOUND,
			err,
		)
	}
}

// TestRead_RefusesWhatAFuzzRunFound proves Read refuses each flow of FUZZ_FOUND with
// ErrForm: they are flows that a fuzz run saw Read return and Emit refuse (B296, Q453).
//
// The check: go test -race -count=1 -run TestRead_RefusesWhatAFuzzRunFound ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 21:23: initial creation (C133, B296, Q453)
func TestRead_RefusesWhatAFuzzRunFound(t *testing.T) {
	for _, flow := range FUZZ_FOUND {
		_, err := runtime.Read([]byte(flow))
		if !errors.Is(
			err,
			runtime.ErrForm,
		) {
			t.Fatalf(
				"%s: got %v, want ErrForm",
				flow,
				err,
			)
		}
	}
}

// BEGIN GOLDEN: no test writes these tables; edit by hand only to review.

// STATES_NOTHING lists a flow with a constant, an argument, an operand or a statement that
// states nothing or is out of range, and the words of Check.
//
// Revisions:
//   - 2026-11-13 20:18: initial creation (C133, B296, Q453)
//
//nolint:lll // a golden row
var (
	STATES_NOTHING = []struct{ name, flow, want string }{
		{
			`a constant with no value and no call`,
			`{"constants": {"A": {}}, "text": "pass"}`,
			`"A": constant has no value and no call: no form`,
		},
		{
			`an argument with no name`,
			`{"args": {"a": {}}, "text": "pass"}`,
			`"a": argument has no name: no form`,
		},
		{
			`an argument with an empty name and a default`,
			`{"args": {"a": {"name": "", "default": {"int": "1"}}}, "text": "pass"}`,
			`"a": argument has no name: no form`,
		},
		{
			`an operand with no source`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], "main": {"statement": [{"call": {"function": "f", "operands": [{}]}}]}}`,
			`call of "f": operand has no source: no form`,
		},
		{
			`a keyword with no value`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], "main": {"statement": [{"call": {"function": "f", "keywords": [{"name": "p"}]}}]}}`,
			`call of "f": operand has no source: no form`,
		},
		{
			`a keyword whose value has no source`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], "main": {"statement": [{"call": {"function": "f", "keywords": [{"name": "p", "value": {}}]}}]}}`,
			`call of "f": operand has no source: no form`,
		},
		{
			`an operand with no source in the call of a constant`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], "constants": {"A": {"call": {"function": "f", "operands": [{}]}}}, "text": "pass"}`,
			`call of "f": operand has no source: no form`,
		},
		{
			`an operand with no source in a call of init`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], "init": [{"call": {"function": "f", "operands": [{}]}}], "text": "pass"}`,
			`call of "f": operand has no source: no form`,
		},
		{
			`a spawn with no call`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"spawn": {}}]}}`,
			`spawn: the call is missing: no form`,
		},
		{
			`a spawn whose call binds a result`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"spawn": {"binding": "h", "call": {"function": "f", "result": "r"}}}]}}`,
			`call of "f": a call here binds no result: no form`,
		},
		{
			`a repeat with no call`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"repeat": {"count": 2}}]}}`,
			`repeat: the call is missing: no form`,
		},
		{
			`a repeat with a count below zero`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"repeat": {"call": {"function": "f"}, "count": -1}}]}}`,
			`repeat: a count, a delay or a timeout is below zero: no form`,
		},
		{
			`a retry with a delay below zero`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"retry": {"call": {"function": "f"}, "attempts": 2, "delay_ms": -1}}]}}`,
			`retry: a count, a delay or a timeout is below zero: no form`,
		},
		{
			`a timeout below zero`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"timeout": {"call": {"function": "f"}, "timeout_ms": -3}}]}}`,
			`timeout: a count, a delay or a timeout is below zero: no form`,
		},
		{
			`a timeout with no call`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"timeout": {}}]}}`,
			`timeout: the call is missing: no form`,
		},
		{
			`a sleep below zero`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"sleep": {"duration_ms": -1}}]}}`,
			`sleep: a duration is below zero: no form`,
		},
		{
			`a match with no cases`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"match": {"expression": {"value": "1"}}}]}}`,
			`match: no cases or no expression: no form`,
		},
		{
			`a match with no expression`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"match": {"expression": {}, "cases": [{"value": "a", "statement": {"call": {"function": "f"}}}]}}]}}`,
			`match: no cases or no expression: no form`,
		},
		{
			`a condition whose call binds a result`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"if": {"condition": {"call": {"function": "f", "result": "r"}}, "then": {"call": {"function": "f"}}}}]}}`,
			`call of "f": a call here binds no result: no form`,
		},
		{
			`a constant whose call binds a result`,
			`{"functions": [{"name": "f", "body": "pass"}], "constants": {"A": {"call": {"function": "f", "result": "r"}}}, "text": "pass"}`,
			`call of "f": a call here binds no result: no form`,
		},
		{
			`an if with no condition`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"if": {"then": {"call": {"function": "f"}}}}]}}`,
			`if: the condition states nothing: no form`,
		},
		{
			`an if whose condition has no kind`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"if": {"condition": {}, "then": {"call": {"function": "f"}}}}]}}`,
			`if: the condition states nothing: no form`,
		},
		{
			`an if whose condition is an empty name, refused as a name`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"if": {"condition": {"name": ""}, "then": {"call": {"function": "f"}}}}]}}`,
			`"": condition is not a Starlark identifier: no form`,
		},
		{
			`a loop with no count`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"loop": {"body": {"call": {"function": "f"}}}}]}}`,
			`loop: the count is not a literal or a name: no form`,
		},
		{
			`a loop whose count has no source`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"loop": {"times": {}, "body": {"call": {"function": "f"}}}}]}}`,
			`loop: the count is not a literal or a name: no form`,
		},
		{
			`a loop whose count is an empty name, refused as an operand`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"loop": {"times": {"name": ""}, "body": {"call": {"function": "f"}}}}]}}`,
			`"": operand is not a Starlark identifier: no form`,
		},
		{
			`a loop whose count is an empty expression`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"loop": {"times": {"expression": ""}, "body": {"call": {"function": "f"}}}}]}}`,
			`loop: the count is not a literal or a name: no form`,
		},
		{
			`a loop whose count is an expression`,
			`{"functions": [{"name": "f", "body": "pass"}], "main": {"statement": [{"loop": {"times": {"expression": "2"}, "body": {"call": {"function": "f"}}}}]}}`,
			`loop: the count is not a literal or a name: no form`,
		},
	}
)

// STATES_SOMETHING lists a flow that states each of those, and is read.
//
// Revisions:
//   - 2026-11-13 20:18: initial creation (C133, B296, Q453)
//
//nolint:lll // a golden row
var (
	STATES_SOMETHING = []struct{ name, flow string }{
		{
			`an argument with no default`,
			`{"args": {"a": {"name": "a"}}, "text": "pass"}`,
		},
		{
			`an argument that binds a name other than its own`,
			`{"args": {"A": {"name": "a"}}, "text": "pass"}`,
		},
		{
			`a constant of a value`,
			`{"constants": {"A": {"value": {"int": "1"}}}, "text": "pass"}`,
		},
		{
			`an operand of each source`,
			`{"functions": [{"name": "f", "params": ["p", "q", "r"], "body": "pass"}], "constants": {"K": {"value": {"int": "1"}}}, "main": {"statement": [{"call": {"function": "f", "operands": [{"literal": {"int": "1"}}, {"name": "K"}, {"expression": "K + 1"}]}}]}}`,
		},
	}
)

// END GOLDEN
