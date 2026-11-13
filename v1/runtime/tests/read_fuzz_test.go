package tests

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/thebagchi/lark/v1/runtime"
)

const (
	// READ_LIMIT is the most bytes of JSON the Read fuzz target takes, so one execution
	// stays short (Q4).
	//
	// Revisions:
	//   - 2026-11-13 17:42: initial creation (C133, Q4, Q444)
	READ_LIMIT = 1 << 16

	// READ_WORDS is the most bytes of error text the Read fuzz target accepts: 100 bytes of
	// quote (Q4) and the fixed words around it.
	//
	// Revisions:
	//   - 2026-11-13 17:42: initial creation (C133, Q4, Q444)
	READ_WORDS = 600
)

// FuzzRead checks, for any bytes, that Read neither panics nor returns a flow with an
// error, that an error is short (Q4), and that a flow it returns is one Check accepts, that
// Emit writes, and that protojson writes so that Read reads it again to an equal flow
// (Q445, B295, B296).
//
// The check: go test -run XXX -fuzz '^FuzzRead$' -fuzztime 60s ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q4, Q444, Q445)
func FuzzRead(f *testing.F) {
	for _, tc := range DECODER {
		f.Add([]byte(tc.flow))
	}

	for _, tc := range CHECKED {
		f.Add([]byte(tc.flow))
	}

	for _, tc := range NULLS {
		f.Add([]byte(tc.flow))
	}

	for _, tc := range UNPARSED {
		f.Add([]byte(tc.flow))
	}

	for _, tc := range PARSED {
		f.Add([]byte(tc.flow))
	}

	for _, tc := range STATES_NOTHING {
		f.Add([]byte(tc.flow))
	}

	for _, tc := range STATES_SOMETHING {
		f.Add([]byte(tc.flow))
	}

	for _, statement := range SHAPES_OF_STATEMENT {
		f.Add([]byte(_Shape(statement)))
	}

	for _, flow := range SHAPES_OF_FLOW {
		f.Add([]byte(flow))
	}

	for _, flow := range EVERY_FLOW {
		f.Add([]byte(flow))
	}

	for _, flow := range FUZZ_FOUND {
		f.Add([]byte(flow))
	}

	for _, seed := range []string{
		READ_FLOW,
		`{}`,
		`[]`,
		`null`,
		`{"text": "pass", "init": null}`,
		`{"constants": {"K": {"value": {"null": null}}}, "text": "pass"}`,
		`{"constants": {"K": {"value": {"float": "NaN"}}}, "text": "pass"}`,
		`{"constants": {"K": {"value": {"bytes": "/w=="}}}, "text": "pass"}`,
		`{"constants": {"K": {"value": {"map": {"entries": {"": {"int": "-0"}}}}}}, ` +
			`"text": "pass"}`,
		`{"main": {"statement": [{"loop": {"times": {"literal": {"int": "3"}}, ` +
			`"body": {"call": {"function": "f"}}}}]}}`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(
		t *testing.T,
		data []byte,
	) {
		if len(data) > READ_LIMIT {
			return
		}

		flow, err := runtime.Read(data)
		if err != nil {
			if flow != nil {
				t.Fatalf(
					"a flow with an error: %v",
					err,
				)
			}

			if len(err.Error()) > READ_WORDS {
				t.Fatalf(
					"the error is %d bytes: %.200s",
					len(err.Error()),
					err,
				)
			}

			return
		}

		err = runtime.Check(flow)
		if err != nil {
			t.Fatalf(
				"Read returned a flow that Check refuses: %v",
				err,
			)
		}

		// Check refuses what Emit has no form for (B295, B296), so Emit writes a flow
		// that Read returned.
		_, err = runtime.Emit(flow)
		if err != nil {
			t.Fatalf(
				"Read returned a flow that Emit refuses: %v",
				err,
			)
		}

		sent, err := protojson.Marshal(flow)
		if err != nil {
			t.Fatalf(
				"a flow Read returned does not write: %v",
				err,
			)
		}

		again, err := runtime.Read(sent)
		if err != nil {
			t.Fatalf(
				"Read does not read what protojson wrote for its flow: %v\n%s",
				err,
				sent,
			)
		}

		if !proto.Equal(
			flow,
			again,
		) {
			t.Fatalf(
				"the flow changes when it is written and read:\n%v\n%v",
				flow,
				again,
			)
		}
	})
}
