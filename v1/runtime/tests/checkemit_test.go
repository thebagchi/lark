package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
)

// _Shape is the flow whose main holds statement, with the functions f and g that the
// shapes call.
//
// Revisions:
//   - 2026-11-13 20:25: initial creation (C133, B296, Q453)
func _Shape(statement string) string {
	return `{"functions": [{"name": "f", "body": "pass"}, {"name": "g", "params": ["p"], ` +
		`"body": "pass"}], "main": {"statement": [` + statement + `]}}`
}

// TestRead_EmitWritesEveryFlowReadReturns proves that, for each flow of the shapes below,
// either Read refuses it or Emit writes it: Check refuses what Emit has no form for
// (B296, Q453). The shapes are the statements of the schema each with a part that
// states nothing or is out of range, and the flows of the constant, the argument and
// the init entry that state nothing.
//
// The check: go test -race -count=1 -run TestRead_EmitWritesEveryFlowReadReturns
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 20:23: initial creation (C133, B296, Q453)
func TestRead_EmitWritesEveryFlowReadReturns(t *testing.T) {
	var flows []string

	for _, statement := range SHAPES_OF_STATEMENT {
		flows = append(
			flows,
			_Shape(statement),
		)
	}

	flows = append(
		flows,
		SHAPES_OF_FLOW...,
	)

	var read int

	for _, flow := range flows {
		parsed, err := runtime.Read([]byte(flow))
		if err != nil {
			continue
		}

		read++

		_, err = runtime.Emit(parsed)
		if err != nil {
			t.Errorf(
				"Read returned a flow that Emit refuses: %v\n%s",
				err,
				flow,
			)
		}
	}

	if read < len(SHAPES_OF_STATEMENT)/4 {
		t.Fatalf(
			"only %d flows were read of %d",
			read,
			len(flows),
		)
	}
}

// BEGIN GOLDEN: no test writes these tables; edit by hand only to review.

// SHAPES_OF_STATEMENT lists a statement of main, each of a kind with a part that states
// nothing, is below zero, or binds a result where the form has none; some are valid.
//
// Revisions:
//   - 2026-11-13 20:23: initial creation (C133, B296, Q453)
//
//nolint:lll // a golden row
var (
	SHAPES_OF_STATEMENT = []string{
		`{"spawn": {}}`,
		`{"spawn": {"binding": "h"}}`,
		`{"spawn": {"call": {"function": "f"}}}`,
		`{"spawn": {"binding": "h", "call": {"function": "f", "result": "r"}}}`,
		`{"join": {}}`,
		`{"cancel": {}}`,
		`{"if": {"condition": {"value": true}, "then": {"call": {"function": "f"}}}}`,
		`{"if": {"condition": {"call": {"function": "f", "result": "r"}}, "then": {"call": {"function": "f"}}}}`,
		`{"if": {"condition": {"call": {"function": "f"}}, "then": {"call": {"function": "f"}}, "else": {"call": {"function": "f"}}}}`,
		`{"match": {}}`,
		`{"match": {"expression": {}}}`,
		`{"match": {"expression": {"value": "1"}}}`,
		`{"match": {"expression": {}, "cases": [{"value": "a", "statement": {"call": {"function": "f"}}}]}}`,
		`{"match": {"expression": {"call": {"function": "f", "result": "r"}}, "cases": [{"value": "a", "statement": {"call": {"function": "f"}}}]}}`,
		`{"match": {"expression": {"value": "1"}, "cases": [{"value": "a", "statement": {"call": {"function": "f"}}}], "default": {"call": {"function": "f"}}}}`,
		`{"repeat": {}}`,
		`{"repeat": {"call": {"function": "f"}}}`,
		`{"repeat": {"call": {"function": "f"}, "count": -1}}`,
		`{"repeat": {"call": {"function": "f"}, "count": 2, "delay_ms": -5}}`,
		`{"repeat": {"call": {"function": "f", "result": "r"}, "count": 2}}`,
		`{"retry": {}}`,
		`{"retry": {"call": {"function": "f"}, "attempts": 0}}`,
		`{"retry": {"call": {"function": "f"}, "attempts": -2}}`,
		`{"retry": {"call": {"function": "f"}, "attempts": 2, "delay_ms": -1}}`,
		`{"retry": {"call": {"function": "f", "result": "r"}, "attempts": 2}}`,
		`{"sleep": {}}`,
		`{"sleep": {"duration_ms": -1}}`,
		`{"timeout": {}}`,
		`{"timeout": {"call": {"function": "f"}}}`,
		`{"timeout": {"call": {"function": "f"}, "timeout_ms": -3}}`,
		`{"timeout": {"call": {"function": "f", "result": "r"}, "timeout_ms": 5}}`,
		`{"loop": {"times": {"literal": {"int": "-1"}}, "body": {"call": {"function": "f"}}}}`,
		`{"loop": {"times": {"literal": {"int": "2"}}, "body": {"call": {"function": "f"}}}}`,
		`{"call": {"function": "f", "result": "r"}}`,
		`{"call": {"function": "g", "operands": [{"expression": "lambda: 1"}]}}`,
	}
)

// SHAPES_OF_FLOW lists a whole flow whose constant, argument or init entry states
// nothing or binds a result where the form has none.
//
// Revisions:
//   - 2026-11-13 20:23: initial creation (C133, B296, Q453)
//
//nolint:lll // a golden row
var (
	SHAPES_OF_FLOW = []string{
		`{"constants": {"A": {}}, "text": "pass"}`,
		`{"constants": {"A": {"call": {"function": "f", "result": "r"}}}, "functions": [{"name": "f", "body": "pass"}], "text": "pass"}`,
		`{"constants": {"A": {"call": {"function": "f"}}}, "functions": [{"name": "f", "body": "pass"}], "text": "pass"}`,
		`{"args": {"a": {}}, "text": "pass"}`,
		`{"args": {"a": {"name": "a"}}, "text": "pass"}`,
		`{"init": [{"call": {"function": "f", "result": "r"}}], "functions": [{"name": "f", "body": "pass"}], "text": "pass"}`,
		`{"init": [{"call": {"function": "f"}}], "functions": [{"name": "f", "body": "pass"}], "text": "pass"}`,
	}
)

// END GOLDEN
