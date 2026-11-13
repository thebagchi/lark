package tests

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// _Flowed is the flow of a script as a .json file holds it: derived, written as
// JSON, read back, and written as a script. The step through JSON is the step a
// file makes, so what the file cannot keep is lost here too.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q422)
func _Flowed(
	t *testing.T,
	source *common.Source,
) (*workflowpb.Flow, []byte) {
	t.Helper()

	flow, err := runtime.Derive(source)
	if err != nil {
		t.Fatalf(
			"derive: %v",
			err,
		)
	}

	sent, err := protojson.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}

	var read = new(workflowpb.Flow)

	err = protojson.Unmarshal(
		sent,
		read,
	)
	if err != nil {
		t.Fatalf(
			"read the flow back from %s: %v",
			sent,
			err,
		)
	}

	err = runtime.Check(read)
	if err != nil {
		t.Fatalf(
			"check: %v",
			err,
		)
	}

	text, err := runtime.Emit(read)
	if err != nil {
		t.Fatalf(
			"emit: %v",
			err,
		)
	}

	return read, text
}

// TestRun_TheScriptAndTheFlowAgreeOnEveryKind proves the script and the flow
// derived from it, sent through JSON, run to one value of one type, for every
// kind of value: in a constant, in an argument of a call, in the default of an
// argument, and at any depth. The kinds that JSON numbers merged are kept apart:
// 1 and 1.0, and the largest int64.
//
// The check: go test -race -count=1 -run TestRun_TheScriptAndTheFlowAgreeOnEveryKind
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421, Q422)
func TestRun_TheScriptAndTheFlowAgreeOnEveryKind(t *testing.T) {
	for _, tc := range AGREED {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var source = &common.Source{
					Entry: "kinds.star",
					Text:  []byte(tc.script),
				}

				want, err := runtime.Run(
					t.Context(),
					source,
				)
				if err != nil {
					t.Fatalf(
						"run the script: %v",
						err,
					)
				}

				if want.String() != tc.value || want.Type() != tc.kind {
					t.Fatalf(
						"the script gives %v (%s), want %s (%s)",
						want,
						want.Type(),
						tc.value,
						tc.kind,
					)
				}

				var _, text = _Flowed(
					t,
					source,
				)

				got, err := runtime.Run(
					t.Context(),
					&common.Source{
						Entry: "kinds_flow.star",
						Text:  text,
					},
				)
				if err != nil {
					t.Fatalf(
						"run the flow: %v",
						err,
					)
				}

				if got.String() != want.String() || got.Type() != want.Type() {
					t.Fatalf(
						"the flow gives %v (%s), the script %v (%s)",
						got,
						got.Type(),
						want,
						want.Type(),
					)
				}
			},
		)
	}
}

// BEGIN GOLDEN: no test writes this table; edit by hand only to review.

// AGREED lists a script, the value it returns as Starlark prints it, and its
// type.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421)
//
//nolint:lll // a golden row
var (
	AGREED = []struct{ name, script, value, kind string }{
		{
			`none`,
			"X = None\ndef main():\n    return X\n",
			`None`,
			`NoneType`,
		},
		{
			`a bool`,
			"X = True\ndef main():\n    return X\n",
			`True`,
			`bool`,
		},
		{
			`an int`,
			"X = 7\ndef main():\n    return X\n",
			`7`,
			`int`,
		},
		{
			`the largest int64, which grows past 64 bits when it is added to`,
			"X = 9223372036854775807\ndef main():\n    return X + 1\n",
			`9223372036854775808`,
			`int`,
		},
		{
			`the int 1 divides as an int`,
			"X = 1\ndef main():\n    return X // 1\n",
			`1`,
			`int`,
		},
		{
			`the float 1.0 divides as a float`,
			"X = 1.0\ndef main():\n    return X // 1\n",
			`1.0`,
			`float`,
		},
		{
			`a float with a fraction`,
			"X = 0.1\ndef main():\n    return X\n",
			`0.1`,
			`float`,
		},
		{
			`a float with an exponent`,
			"X = 1e21\ndef main():\n    return X\n",
			`1e+21`,
			`float`,
		},
		{
			`a string`,
			"X = \"a\\\"b\\nc é\"\ndef main():\n    return X\n",
			`"a\"b\nc é"`,
			`string`,
		},
		{
			`bytes`,
			"X = b\"0\"\ndef main():\n    return X\n",
			`b"0"`,
			`bytes`,
		},
		{
			`bytes that are not UTF-8`,
			"X = b\"\\xff\\x00\\\"\\\\\"\ndef main():\n    return X\n",
			`b"\xff\x00\"\\"`,
			`bytes`,
		},
		{
			`a list of every scalar`,
			"X = [None, True, 1, 1.0, \"s\", b\"b\"]\ndef main():\n    return X\n",
			`[None, True, 1, 1.0, "s", b"b"]`,
			`list`,
		},
		{
			`lists and a map inside each other`,
			"X = [[1, [b\"a\"]], {\"k\": [{\"j\": 2.5}]}, []]\ndef main():\n    return X\n",
			`[[1, [b"a"]], {"k": [{"j": 2.5}]}, []]`,
			`list`,
		},
		{
			`a map with a bytes value`,
			"X = {\"k\": b\"\\xfe\"}\ndef main():\n    return X[\"k\"]\n",
			`b"\xfe"`,
			`bytes`,
		},
		{
			`an argument of a call in main`,
			"def f(a, b, c):\n    return [c, b, a]\ndef main():\n    return f(b\"\\xff\", 1.0, [1, 18446744073709551617])\n",
			`[[1, 18446744073709551617], 1.0, b"\xff"]`,
			`list`,
		},
		{
			`an argument of a call at the top`,
			"S = []\ndef f(a):\n    S.append(a)\nf(b\"\\xff\")\nf(1.0)\ndef main():\n    return S\n",
			`[b"\xff", 1.0]`,
			`list`,
		},
		{
			`a call constant with a bytes argument`,
			"def f(a):\n    return a\nX = f(b\"x\")\ndef main():\n    return X\n",
			`b"x"`,
			`bytes`,
		},
		{
			`an argument with a bytes default`,
			"a = arg(\"a\", b\"dflt\")\ndef main():\n    return a\n",
			`b"dflt"`,
			`bytes`,
		},
		{
			`an argument with a float default`,
			"a = arg(\"a\", 2.0)\ndef main():\n    return a\n",
			`2.0`,
			`float`,
		},
		{
			`a dict with a key that is no string`,
			"X = {1: \"x\"}\ndef main():\n    return X\n",
			`{1: "x"}`,
			`dict`,
		},
	}
)

// END GOLDEN

// TestDerive_ListsAnArgumentWithABytesDefault proves a script that declares
// arg("a", b"d") has the argument in flow.args with the bytes as its default,
// where the flow once kept the declaration as init text (Q407), and a value
// given at run time wins over the default in the script and in the flow.
//
// The check: go test -race -count=1 -run TestDerive_ListsAnArgumentWithABytesDefault
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407)
func TestDerive_ListsAnArgumentWithABytesDefault(t *testing.T) {
	var source = &common.Source{
		Entry: "arg.star",
		Text:  []byte("a = arg(\"a\", b\"d\\xff\")\ndef main():\n    return a\n"),
	}

	var flow, text = _Flowed(
		t,
		source,
	)

	listed := flow.GetArgs()["a"]
	if listed == nil || string(listed.GetDefault().GetBytes()) != "d\xff" ||
		listed.GetName() != "a" {
		t.Fatalf(
			"flow.args holds %v, want a with the default bytes d\\xff",
			flow.GetArgs(),
		)
	}

	if len(flow.GetInit()) != 0 {
		t.Fatalf(
			"the flow holds init %v, want none",
			flow.GetInit(),
		)
	}

	for _, tc := range []struct {
		name  string
		args  map[string]any
		value string
		kind  string
	}{
		{
			"the default",
			nil,
			`b"d\xff"`,
			"bytes",
		},
		{
			"a given value",
			map[string]any{"a": "given"},
			`"given"`,
			"string",
		},
	} {
		for _, run := range []struct {
			name   string
			source *common.Source
		}{
			{
				"script",
				source,
			},
			{
				"flow",
				&common.Source{
					Entry: "arg_flow.star",
					Text:  text,
				},
			},
		} {
			got, err := runtime.Run(
				t.Context(),
				run.source,
				runtime.WithArgs(tc.args),
			)
			if err != nil {
				t.Fatalf(
					"%s, %s: %v",
					tc.name,
					run.name,
					err,
				)
			}

			if got.String() != tc.value || got.Type() != tc.kind {
				t.Fatalf(
					"%s, %s gives %v (%s), want %s (%s)",
					tc.name,
					run.name,
					got,
					got.Type(),
					tc.value,
					tc.kind,
				)
			}
		}
	}
}

// TestRun_EveryByteSurvivesTheFlow proves a bytes value of all 256 byte values,
// sent as the base64 JSON holds, runs as the same 256 bytes.
//
// The check: go test -race -count=1 -run TestRun_EveryByteSurvivesTheFlow
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q422)
func TestRun_EveryByteSurvivesTheFlow(t *testing.T) {
	var all = make(
		[]byte,
		0,
		256,
	)

	for one := range 256 {
		all = append(
			all,
			byte(one),
		)
	}

	var flow = &workflowpb.Flow{
		Constants: map[string]*workflowpb.Constant{
			"X": {
				Kind: &workflowpb.Constant_Value{
					Value: &workflowpb.Value{
						Kind: &workflowpb.Value_Bytes{
							Bytes: all,
						},
					},
				},
			},
		},
		Spine: &workflowpb.Flow_Text{
			Text: "return X == bytes(range(256))",
		},
	}

	sent, err := protojson.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}

	var read = new(workflowpb.Flow)

	err = protojson.Unmarshal(
		sent,
		read,
	)
	if err != nil {
		t.Fatal(err)
	}

	text, err := runtime.Emit(read)
	if err != nil {
		t.Fatal(err)
	}

	got, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "bytes_flow.star",
			Text:  text,
		},
	)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	if got.String() != "True" {
		t.Fatalf(
			"the flow's bytes differ from the 256 bytes: %v\n%s",
			got,
			text,
		)
	}
}
