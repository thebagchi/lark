package tests

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/internal/reader"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// TestNulled_WalksTheFlowBySchema proves reader.Nulled finds a null where a value
// stands only by the schema: a member that is no field is left to the decoder, a
// document that is no object is left too, text that is no JSON is an error, and the
// path in the words is made of schema names and cut to 100 bytes (Q4, Q439).
//
// The check: go test -race -count=1 -run TestNulled_WalksTheFlowBySchema
// ./v1/runtime/internal/reader/tests
//
// Revisions:
//   - 2026-11-13 13:25: initial creation (C132, Q4, Q439)
//   - 2026-11-13 17:39: moved from cmd/lark/tests (C133, Q441, Q447)
func TestNulled_WalksTheFlowBySchema(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		err  bool
	}{
		{
			"a member that is no field",
			`{"nothing": null, "text": "pass"}`,
			false,
		},
		{
			"a document that is a list",
			`[null]`,
			false,
		},
		{
			"a document that is a number",
			`5`,
			false,
		},
		{
			"text that is no JSON",
			`{bad`,
			true,
		},
		{
			"a value that is null in a field of a message",
			`{"args": {"a": {"default": null}}}`,
			true,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := reader.Nulled([]byte(tc.json))
				if (err != nil) != tc.err {
					t.Fatalf(
						"err = %v, want an error: %v",
						err,
						tc.err,
					)
				}
			},
		)
	}

	var deep = `null`

	for range 1000 {
		deep = `{"list": {"items": [` + deep + `]}}`
	}

	err := reader.Nulled([]byte(
		`{"constants": {"K": {"value": ` + deep + `}}}`,
	))
	if err == nil {
		t.Fatal("want an error for a null in a deep list")
	}

	if len(err.Error()) > 400 {
		t.Fatalf(
			"the error is %d bytes, want at most 400",
			len(err.Error()),
		)
	}

	var kind = `{"null": 5}`

	for range 3000 {
		kind = `{"list": {"items": [` + kind + `]}}`
	}

	err = reader.Nulled([]byte(
		`{"constants": {"K": {"value": ` + kind + `}}}`,
	))
	if err == nil {
		t.Fatal("want an error for a null kind that holds 5, 3,000 lists down")
	}

	if len(err.Error()) > 400 {
		t.Fatalf(
			"the error is %d bytes, want at most 400",
			len(err.Error()),
		)
	}
}

// TestNulled_EveryFieldThatLeadsToAValueHasOneName proves the claim the walk of Nulled
// rests on: every field of the schema that holds a workflow.Value, or a message that
// holds one at any depth, has the same name in the .proto and in JSON. protojson takes
// either name, and Nulled looks up the JSON one, so a field with two names on a way to
// a value would hide a null from it (Q439).
//
// The check: go test -race -count=1 -run TestNulled_EveryFieldThatLeadsToAValueHasOneName
// ./v1/runtime/internal/reader/tests
//
// Revisions:
//   - 2026-11-13 17:16: initial creation (C132, Q439)
//   - 2026-11-13 17:39: moved from cmd/lark/tests (C133, Q441, Q447)
func TestNulled_EveryFieldThatLeadsToAValueHasOneName(t *testing.T) {
	var (
		leads = map[protoreflect.FullName]bool{}
		walk  func(message protoreflect.MessageDescriptor) bool
	)

	walk = func(message protoreflect.MessageDescriptor) bool {
		if known, ok := leads[message.FullName()]; ok {
			return known
		}

		// A message on the way is counted as not leading while it is walked, so a
		// message that holds itself ends the walk.
		leads[message.FullName()] = false

		var reaches = message.FullName() == "workflow.Value"

		for index := range message.Fields().Len() {
			var (
				field  = message.Fields().Get(index)
				target = field.Message()
			)

			if field.IsMap() {
				target = field.MapValue().Message()
			}

			if target == nil || !walk(target) {
				continue
			}

			reaches = true

			if field.JSONName() != string(field.Name()) {
				t.Errorf(
					"%s.%s leads to a value and has two names: %s and %s",
					message.FullName(),
					field.Name(),
					field.Name(),
					field.JSONName(),
				)
			}
		}

		leads[message.FullName()] = reaches

		return reaches
	}

	if !walk(new(workflowpb.Flow).ProtoReflect().Descriptor()) {
		t.Fatal("the Flow leads to no value")
	}
}

// TestNulled_BoundsItsOwnDepth proves the walk refuses what nests past common.DEPTH
// messages with trail.ErrDeep and a short error, and reads the same flow when it is
// within the bound, since the walk holds no recursion over JSON it skips (Q2). A flow
// that protojson has read never reaches the bound; this is the guard for a direct call.
//
// The check: go test -race -count=1 -run TestNulled_BoundsItsOwnDepth
// ./v1/runtime/internal/reader/tests
//
// Revisions:
//   - 2026-11-13 19:34: initial creation (C133, Q2)
func TestNulled_BoundsItsOwnDepth(t *testing.T) {
	var nest = func(count int) string {
		return `{"constants": {"K": {"value": ` + strings.Repeat(
			`{"list": {"items": [`,
			count,
		) + `{"null": null}` + strings.Repeat(
			`]}}`,
			count,
		) + `}}}`
	}

	err := reader.Nulled([]byte(nest(4000)))
	if err != nil {
		t.Fatalf(
			"4,000 lists: %v",
			err,
		)
	}

	err = reader.Nulled([]byte(nest(100000)))
	if !errors.Is(
		err,
		trail.ErrDeep,
	) {
		t.Fatalf(
			"100,000 lists: got %v, want %v",
			err,
			trail.ErrDeep,
		)
	}

	if len(err.Error()) > 400 {
		t.Fatalf(
			"the error is %d bytes, want at most 400",
			len(err.Error()),
		)
	}

	// Unknown members and arrays are skipped without recursion.
	var skipped = `{"nothing": ` + strings.Repeat(
		"[",
		1000000,
	) + strings.Repeat(
		"]",
		1000000,
	) + `, "text": "pass"}`

	err = reader.Nulled([]byte(skipped))
	if err != nil {
		t.Fatalf(
			"a skipped array of a million levels: %v",
			err,
		)
	}
}

// TestNulled_RefusesJSONThatIsCut proves the walk returns the error of the JSON, and
// not a null it did not see, when the text ends inside a value, a member, a list, a map,
// a skipped member or a null kind, and when data follows the value. Read never gets
// here, since protojson refuses such text first; this is the guard for a direct call.
//
// The check: go test -race -count=1 -run TestNulled_RefusesJSONThatIsCut
// ./v1/runtime/internal/reader/tests
//
// Revisions:
//   - 2026-11-13 19:50: initial creation (C133, Q2)
func TestNulled_RefusesJSONThatIsCut(t *testing.T) {
	for _, tc := range []struct{ name, json string }{
		{
			"inside the top object",
			`{`,
		},
		{
			"after a key",
			`{"constants"`,
		},
		{
			"before a value",
			`{"constants": `,
		},
		{
			"inside a map",
			`{"constants": {"K": `,
		},
		{
			"inside an entry key of a map of values",
			`{"constants": {"K": {"value": {"map": {"entries": {"k"`,
		},
		{
			"inside the entries of a map",
			`{"constants": {"K": {"value": {"map": {"entries": {`,
		},
		{
			"inside a list",
			`{"constants": {"K": {"value": {"list": {"items": [`,
		},
		{
			"inside a list after an item",
			`{"constants": {"K": {"value": {"list": {"items": [{"int": "1"},`,
		},
		{
			"inside a value",
			`{"constants": {"K": {"value": {`,
		},
		{
			"before the member of a null kind",
			`{"constants": {"K": {"value": {"null": `,
		},
		{
			"inside a member that is skipped",
			`{"nothing": [1, {"a": `,
		},
		{
			"a member of a scalar field",
			`{"text": [`,
		},
		{
			"a list field that is no list",
			`{"functions": {"a": [`,
		},
		{
			"a map field that is no map",
			`{"constants": [{"a": `,
		},
		{
			"before a value that is skipped",
			`{"nothing": `,
		},
		{
			"before the items of a list",
			`{"functions": `,
		},
		{
			"a key of a map that is no string",
			`{"constants": {1: `,
		},
		{
			"another value after the value",
			`{} {}`,
		},
		{
			"text after the value",
			`{} x`,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := reader.Nulled([]byte(tc.json))
				if err == nil {
					t.Fatal("want an error")
				}

				if len(err.Error()) > 400 {
					t.Fatalf(
						"the error is %d bytes",
						len(err.Error()),
					)
				}
			},
		)
	}
}
