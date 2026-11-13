package tests

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// VALUE_LIMIT is the most bytes of JSON the value fuzz target reads, so one
	// execution stays short (Q4).
	//
	// Revisions:
	//   - 2026-11-13 10:15: initial creation (C132, Q4, Q407)
	VALUE_LIMIT = 4096
)

// FuzzValueReader checks, for any JSON text that protojson reads as a Value,
// that Check and Emit agree on it: a value Check refuses is refused by Emit with
// ErrForm or trail.ErrDeep, and a value Check accepts is written as a script
// that parses and reads as the constant X. When the script derives to a flow,
// that flow writes the same script again.
//
// The check: go test -run XXX -fuzz '^FuzzValueReader$' -fuzztime 60s
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q2, Q407, Q421, Q422)
func FuzzValueReader(f *testing.F) {
	for _, tc := range KINDS {
		f.Add(tc.want)
	}

	for _, tc := range LITERALS {
		f.Add(tc.value)
	}

	for _, seed := range []string{
		`{}`,
		`{"int": ""}`,
		`{"int": "-"}`,
		`{"int": "007"}`,
		`{"int": "1); x("}`,
		`{"float": "NaN"}`,
		`{"float": "-Infinity"}`,
		`{"list": {"items": [{}]}}`,
		`{"map": {"entries": {"k": {"map": {"entries": {"": {"null": null}}}}}}}`,
		`{"bytes": "AAECAwQF/f7/"}`,
		`{"string": "\u0000😀"}`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		if len(text) > VALUE_LIMIT {
			return
		}

		var (
			value = new(workflowpb.Value)
			err   = protojson.Unmarshal(
				[]byte(text),
				value,
			)
		)
		if err != nil {
			return
		}

		flow := &workflowpb.Flow{
			Constants: map[string]*workflowpb.Constant{
				"X": {
					Kind: &workflowpb.Constant_Value{
						Value: value,
					},
				},
			},
			Spine: &workflowpb.Flow_Text{
				Text: "print(X)",
			},
		}

		var checked = graph.Check(flow)

		out, err := graph.Emit(flow)

		if checked != nil {
			if !errors.Is(
				checked,
				graph.ErrForm,
			) && !errors.Is(
				checked,
				trail.ErrDeep,
			) {
				t.Fatalf(
					"check refused %q with %v, want ErrForm or ErrDeep",
					text,
					checked,
				)
			}

			if err == nil {
				t.Fatalf(
					"check refused %q and emit wrote it:\n%s",
					text,
					out,
				)
			}

			return
		}

		if err != nil {
			t.Fatalf(
				"check accepted %q and emit refused: %v",
				text,
				err,
			)
		}

		_, err = common.OPTIONS.Parse(
			"fuzz.star",
			out,
			0,
		)
		if err != nil {
			t.Fatalf(
				"check accepted %q and the script does not parse: %v\n%s",
				text,
				err,
				out,
			)
		}

		again, err := graph.Of(&common.Source{
			Entry: "fuzz.star",
			Text:  out,
		})
		if err != nil {
			return
		}

		second, err := graph.Emit(again)
		if err != nil {
			t.Fatalf(
				"emit of the derived flow of %q: %v\n%s",
				text,
				err,
				out,
			)
		}

		if string(second) != string(out) {
			t.Fatalf(
				"emit of %q is not a fixed point\n--- first\n%s\n--- second\n%s",
				text,
				out,
				second,
			)
		}
	})
}
