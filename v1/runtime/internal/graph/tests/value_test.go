package tests

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// VALUE_MAIN is the entry that reads the constant X, so the shake keeps it.
	//
	// Revisions:
	//   - 2026-11-13 10:15: initial creation (C132, Q407)
	VALUE_MAIN = `
def main():
    print(X)
`

	// BAD_DEPTH is how many lists deep a value is built to be refused: one more
	// than the depth every walk accepts (common.DEPTH, Q2).
	//
	// Revisions:
	//   - 2026-11-13 10:15: initial creation (C132, Q2, Q407)
	BAD_DEPTH = 10002
)

// TestOf_TagsEveryKindOfValue proves a value a script states is a tagged value
// in the flow, and the JSON of each kind is the object named for it: an int is
// an int64, written as text, a float is a float, so 1 and 1.0 are two values,
// bytes are base64 of the bytes whatever they are, a map keeps its string keys.
//
// The check: go test -count=1 -run TestOf_TagsEveryKindOfValue
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421, Q422)
func TestOf_TagsEveryKindOfValue(t *testing.T) {
	for _, tc := range KINDS {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					flow = _Derived(
						t,
						tc.script+VALUE_MAIN,
					)
					raw, err = protojson.Marshal(flow)
				)
				if err != nil {
					t.Fatal(err)
				}

				var (
					doc  map[string]any
					want any
				)

				err = json.Unmarshal(
					raw,
					&doc,
				)
				if err != nil {
					t.Fatal(err)
				}

				err = json.Unmarshal(
					[]byte(tc.want),
					&want,
				)
				if err != nil {
					t.Fatalf(
						"golden %q: %v",
						tc.want,
						err,
					)
				}

				var (
					constants, _ = doc["constants"].(map[string]any)
					held, _      = constants["X"].(map[string]any)
				)

				if !reflect.DeepEqual(
					held["value"],
					want,
				) {
					t.Fatalf(
						"the value of X in %s is %v, want %s",
						raw,
						held["value"],
						tc.want,
					)
				}
			},
		)
	}
}

// BEGIN GOLDEN: no test writes this table; edit by hand only to review.

// KINDS lists a script that states the constant X and the JSON of the value the
// flow holds for it.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421, Q422)
//
//nolint:lll // a golden row
var (
	KINDS = []struct{ name, script, want string }{
		{
			`none`,
			`X = None`,
			`{"null": null}`,
		},
		{
			`true`,
			`X = True`,
			`{"bool": true}`,
		},
		{
			`false`,
			`X = False`,
			`{"bool": false}`,
		},
		{
			`an int`,
			`X = 3`,
			`{"int": "3"}`,
		},
		{
			`zero`,
			`X = 0`,
			`{"int": "0"}`,
		},
		{
			`the largest int64 is kept exactly`,
			`X = 9223372036854775807`,
			`{"int": "9223372036854775807"}`,
		},
		{
			`an int past 2^53 is kept exactly`,
			`X = 9007199254740993`,
			`{"int": "9007199254740993"}`,
		},
		{
			`a whole float is a float`,
			`X = 1.0`,
			`{"float": 1}`,
		},
		{
			`a float with a fraction`,
			`X = 1.5`,
			`{"float": 1.5}`,
		},
		{
			`a float with an exponent`,
			`X = 1e21`,
			`{"float": 1e+21}`,
		},
		{
			`a string`,
			`X = "x"`,
			`{"string": "x"}`,
		},
		{
			`a string with a quote, a line break and a wide letter`,
			`X = "a\"b\nc é"`,
			`{"string": "a\"b\nc é"}`,
		},
		{
			`bytes`,
			`X = b"0"`,
			`{"bytes": "MA=="}`,
		},
		{
			`bytes that are not UTF-8`,
			`X = b"\xff"`,
			`{"bytes": "/w=="}`,
		},
		{
			`no bytes`,
			`X = b""`,
			`{"bytes": ""}`,
		},
		{
			`an empty list`,
			`X = []`,
			`{"list": {}}`,
		},
		{
			`a list of every scalar kind`,
			`X = [None, True, 1, 1.0, "s", b"b"]`,
			`{"list": {"items": [{"null": null}, {"bool": true}, {"int": "1"}, {"float": 1}, {"string": "s"}, {"bytes": "Yg=="}]}}`,
		},
		{
			`an empty map`,
			`X = {}`,
			`{"map": {}}`,
		},
		{
			`a map of one pair`,
			`X = {"k": 1}`,
			`{"map": {"entries": {"k": {"int": "1"}}}}`,
		},
		{
			`lists and maps inside each other`,
			`X = [[1, [b"a"]], {"k": [{"j": 2.5}]}, []]`,
			`{"list": {"items": [{"list": {"items": [{"int": "1"}, {"list": {"items": [{"bytes": "YQ=="}]}}]}}, {"map": {"entries": {"k": {"list": {"items": [{"map": {"entries": {"j": {"float": 2.5}}}}]}}}}}, {"list": {}}]}}`,
		},
	}
)

// END GOLDEN

// TestOf_KeepsOneAndOnePointZeroApart proves the int 1 and the float 1.0 are two
// values in the flow, and that each is written back as the literal it came from.
//
// The check: go test -count=1 -run TestOf_KeepsOneAndOnePointZeroApart
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q421)
func TestOf_KeepsOneAndOnePointZeroApart(t *testing.T) {
	var (
		whole = _Derived(
			t,
			"X = 1"+VALUE_MAIN,
		)
		fraction = _Derived(
			t,
			"X = 1.0"+VALUE_MAIN,
		)
	)

	if proto.Equal(
		whole,
		fraction,
	) {
		t.Fatalf(
			"1 and 1.0 gave one flow: %v",
			whole,
		)
	}

	for text, flow := range map[string]*workflowpb.Flow{
		"X = 1\n":   whole,
		"X = 1.0\n": fraction,
	} {
		out, err := graph.Emit(flow)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(
			string(out),
			text,
		) {
			t.Fatalf(
				"want %q in %q",
				text,
				out,
			)
		}
	}
}

// TestEmit_IsAFixedPointOfOfOnEveryKind proves a flow survives JSON, is accepted
// by Check, and writes a script that derives to a flow that writes the same
// script again, for every kind of value, in a constant, an argument default, a
// call argument of main and a call argument of the init.
//
// The check: go test -count=1 -run TestEmit_IsAFixedPointOfOfOnEveryKind
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421, Q422)
func TestEmit_IsAFixedPointOfOfOnEveryKind(t *testing.T) {
	var scripts = []string{
		"X = None\n" + VALUE_MAIN,
		"X = True\n" + VALUE_MAIN,
		"X = 9223372036854775807\n" + VALUE_MAIN,
		"X = [1, 1.0, 2.5, 1e21]\n" + VALUE_MAIN,
		"X = b\"\\xff\\x00\\\"\\\\\"\n" + VALUE_MAIN,
		"X = [[1, [b\"a\"]], {\"k\": [{\"j\": b\"\"}]}, []]\n" + VALUE_MAIN,
		"X = arg(\"X\", b\"\\xfe\")\n" + VALUE_MAIN,
		"X = arg(\"X\", [1, {\"k\": 1.0}])\n" + VALUE_MAIN,
		"X = arg(\"X\")\n" + VALUE_MAIN,
		"def f(a, b, c):\n    return a\nX = f(b\"\\x80\", 1.0, [1])\n" + VALUE_MAIN,
		"def f(a, b):\n    return a\ndef main():\n    f(b\"\\xff\", {\"k\": [1, 1.0]})\n",
		"def f(a, b):\n    return a\nf(b\"\\xff\", [None])\ndef main():\n    pass\n",
	}

	for _, script := range scripts {
		t.Run(
			script,
			func(t *testing.T) {
				var (
					first = _Derived(
						t,
						script,
					)
					raw, err = protojson.Marshal(first)
				)
				if err != nil {
					t.Fatal(err)
				}

				read := _Flow(
					t,
					string(raw),
				)

				if !proto.Equal(
					first,
					read,
				) {
					t.Fatalf(
						"JSON changed the flow:\n%v\n%v",
						first,
						read,
					)
				}

				err = graph.Check(read)
				if err != nil {
					t.Fatalf(
						"check: %v",
						err,
					)
				}

				text1, err := graph.Emit(read)
				if err != nil {
					t.Fatal(err)
				}

				var second = _Derived(
					t,
					string(text1),
				)

				text2, err := graph.Emit(second)
				if err != nil {
					t.Fatal(err)
				}

				if string(text1) != string(text2) {
					t.Fatalf(
						"emit is not a fixed point:\n%s\n%s",
						text1,
						text2,
					)
				}

				if !proto.Equal(
					read,
					second,
				) {
					t.Fatalf(
						"the flow of the emitted script differs:\n%v\n%v",
						read,
						second,
					)
				}
			},
		)
	}
}

// TestEmit_WritesEveryKindAsTheLiteralItIs proves a value of a flow is written as
// the Starlark literal of its kind, whatever the bytes or the digits are.
//
// The check: go test -count=1 -run TestEmit_WritesEveryKindAsTheLiteralItIs
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421, Q422)
func TestEmit_WritesEveryKindAsTheLiteralItIs(t *testing.T) {
	for _, tc := range LITERALS {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					flow = _Flow(
						t,
						`{"constants": {"X": {"value": `+tc.value+`}}, `+
							`"main": {"statement": [{"call": `+
							`{"function": "print", "builtin": true, `+
							`"operands": [{"name": "X"}]}}]}}`,
					)
					err = graph.Check(flow)
				)
				if err != nil {
					t.Fatalf(
						"check: %v",
						err,
					)
				}

				out, err := graph.Emit(flow)
				if err != nil {
					t.Fatal(err)
				}

				if !strings.HasPrefix(
					string(out),
					"X = "+tc.want+"\n",
				) {
					t.Fatalf(
						"want X = %s first in %q",
						tc.want,
						out,
					)
				}
			},
		)
	}
}

// BEGIN GOLDEN: no test writes this table; edit by hand only to review.

// LITERALS lists the JSON of a value and the literal the flow writes for it.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421, Q422)
//
//nolint:lll // a golden row
var (
	LITERALS = []struct{ name, value, want string }{
		{
			`null`,
			`{"null": null}`,
			`None`,
		},
		{
			`true`,
			`{"bool": true}`,
			`True`,
		},
		{
			`a negative int`,
			`{"int": "-5"}`,
			`-5`,
		},
		{
			`zero`,
			`{"int": "0"}`,
			`0`,
		},
		{
			`the smallest int64`,
			`{"int": "-9223372036854775808"}`,
			`-9223372036854775808`,
		},
		{
			`the largest int64`,
			`{"int": "9223372036854775807"}`,
			`9223372036854775807`,
		},
		{
			`an int written as a number`,
			`{"int": 12}`,
			`12`,
		},
		{
			`a whole float keeps its point`,
			`{"float": 3}`,
			`3.0`,
		},
		{
			`a float with a fraction`,
			`{"float": 0.1}`,
			`0.1`,
		},
		{
			`a float with a positive exponent`,
			`{"float": 1e21}`,
			`1e+21`,
		},
		{
			`a float with a negative exponent`,
			`{"float": 1e-7}`,
			`1e-07`,
		},
		{
			`the smallest float`,
			`{"float": 5e-324}`,
			`5e-324`,
		},
		{
			`negative zero`,
			`{"float": -0.0}`,
			`-0.0`,
		},
		{
			`a string with a quote and a line break`,
			`{"string": "a\"b\nc"}`,
			`"a\"b\nc"`,
		},
		{
			`bytes that are not UTF-8`,
			`{"bytes": "/w=="}`,
			`b"\xff"`,
		},
		{
			`bytes with a quote and a backslash`,
			`{"bytes": "IlxB"}`,
			`b"\x22\x5cA"`,
		},
		{
			`bytes with a line break and a zero`,
			`{"bytes": "CgA="}`,
			`b"\x0a\x00"`,
		},
		{
			`no bytes`,
			`{"bytes": ""}`,
			`b""`,
		},
		{
			`a list`,
			`{"list": {"items": [{"int": "1"}, {"float": 1}, {"bytes": "YQ=="}]}}`,
			`[1, 1.0, b"a"]`,
		},
		{
			`an empty list`,
			`{"list": {}}`,
			`[]`,
		},
		{
			`a map, its keys in order`,
			`{"map": {"entries": {"b": {"int": "2"}, "a": {"int": "1"}}}}`,
			`{"a": 1, "b": 2}`,
		},
		{
			`an empty map`,
			`{"map": {}}`,
			`{}`,
		},
	}
)

// END GOLDEN

// TestCheck_RefusesAMalformedValue proves a value that is no value is refused by
// Check, and by Emit when Check was not asked, with ErrForm: a Value with no kind
// set anywhere a value stands, a null kind that holds anything but null, and a
// float that is NaN or an infinity. A flow is untrusted input, so no text of it
// may reach the script it writes. An int outside int64 or with no digits does not
// get this far: protojson refuses it (TestValue_JSONRefusesAnIntOutsideInt64).
//
// The rules and the clauses they come from:
//   - a float has a literal only when it is finite (the Starlark specification
//     has no literal for NaN and the infinities; protojson accepts the strings
//     "NaN", "Infinity" and "-Infinity" for a double);
//   - a null kind holds null: protojson reads any number for the enum, and 0 is the
//     only one that is null (Q439);
//   - a Value with no kind says nothing (Q407).
//
// The check: go test -count=1 -run TestCheck_RefusesAMalformedValue
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q4, Q407, Q421)
func TestCheck_RefusesAMalformedValue(t *testing.T) {
	var (
		wrap = func(value string) string {
			return `{"constants": {"X": {"value": ` + value + `}}, "text": "pass"}`
		}
		cases = []struct{ name, flow string }{
			{
				"a constant with no kind",
				wrap(`{}`),
			},
			{
				"an item of a list with no kind",
				wrap(`{"list": {"items": [{"int": "1"}, {}]}}`),
			},
			{
				"an entry of a map with no kind",
				wrap(`{"map": {"entries": {"k": {}}}}`),
			},
			{
				"a value two levels down with no kind",
				wrap(`{"list": {"items": [{"map": {"entries": {"k": {}}}}]}}`),
			},
			{
				"the default of an argument with no kind",
				`{"args": {"a": {"name": "a", "default": {}}}, "text": "pass"}`,
			},
			{
				"an argument of a call with no kind",
				`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}],
					"main": {"statement": [{"call": {"function": "f",
						"args": [{}]}}]}}`,
			},
			{
				"an argument of a call of the init with no kind",
				`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}],
					"init": [{"call": {"function": "f", "args": [{}]}}],
					"text": "pass"}`,
			},
			{
				"a literal operand with no kind",
				`{"functions": [{"name": "f", "params": ["p", "q"],
					"body": "pass"}],
					"args": {"q": {"name": "q"}},
					"main": {"statement": [{"call": {"function": "f",
						"operands": [{"literal": {}}, {"name": "q"}]}}]}}`,
			},
			{
				"a keyword value with no kind",
				`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}],
					"main": {"statement": [{"call": {"function": "f",
						"keywords": [{"name": "p",
							"value": {"literal": {}}}]}}]}}`,
			},
			{
				"the count of a loop with no kind",
				`{"functions": [{"name": "f", "body": "pass"}],
					"main": {"statement": [{"loop": {"times": {"literal": {}},
						"body": {"call": {"function": "f"}}}}]}}`,
			},
			{
				"the count of a loop that is a float",
				`{"functions": [{"name": "f", "body": "pass"}],
					"main": {"statement": [{"loop": {
						"times": {"literal": {"float": 2}},
						"body": {"call": {"function": "f"}}}}]}}`,
			},
			{
				"the count of a loop that is a string",
				`{"functions": [{"name": "f", "body": "pass"}],
					"main": {"statement": [{"loop": {
						"times": {"literal": {"string": "2"}},
						"body": {"call": {"function": "f"}}}}]}}`,
			},
			{
				"a null kind that holds something else than null",
				wrap(`{"null": 5}`),
			},
			{
				"a null kind inside a list",
				wrap(`{"list": {"items": [{"null": 5}]}}`),
			},
			{
				"a float that is NaN",
				wrap(`{"float": "NaN"}`),
			},
			{
				"a float that is infinite",
				wrap(`{"float": "Infinity"}`),
			},
			{
				"a float that is negative infinity",
				wrap(`{"float": "-Infinity"}`),
			},
			{
				"a float that is NaN inside a map",
				wrap(`{"map": {"entries": {"k": {"float": "NaN"}}}}`),
			},
		}
	)

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					flow = _Flow(
						t,
						tc.flow,
					)
					err = graph.Check(flow)
				)
				if !errors.Is(
					err,
					graph.ErrForm,
				) {
					t.Fatalf(
						"check: err = %v, want %v",
						err,
						graph.ErrForm,
					)
				}

				_, err = graph.Emit(flow)
				if !errors.Is(
					err,
					graph.ErrForm,
				) {
					t.Fatalf(
						"emit: err = %v, want %v",
						err,
						graph.ErrForm,
					)
				}
			},
		)
	}
}

// TestCheck_RefusesAValueNestedTooDeep proves a value nested past common.DEPTH is
// refused with trail.ErrDeep by Check and by Emit, and one nested a hundred deep
// is not. The value is built in Go: protojson stops a JSON text at its own depth.
//
// The clause is Q2: every recursive Go walk over a value has the depth limit.
//
// The check: go test -count=1 -run TestCheck_RefusesAValueNestedTooDeep
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q2, Q407)
func TestCheck_RefusesAValueNestedTooDeep(t *testing.T) {
	for _, tc := range []struct {
		name  string
		depth int
		err   error
	}{
		{
			"a hundred deep",
			100,
			nil,
		},
		{
			"as deep as JSON can say",
			5000,
			nil,
		},
		{
			"past the limit",
			BAD_DEPTH,
			trail.ErrDeep,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var value = &workflowpb.Value{
					Kind: &workflowpb.Value_Bool{
						Bool: true,
					},
				}

				for range tc.depth {
					value = &workflowpb.Value{
						Kind: &workflowpb.Value_List{
							List: &workflowpb.List{
								Items: []*workflowpb.Value{value},
							},
						},
					}
				}

				var (
					flow = &workflowpb.Flow{
						Constants: map[string]*workflowpb.Constant{
							"X": {
								Kind: &workflowpb.Constant_Value{
									Value: value,
								},
							},
						},
						Spine: &workflowpb.Flow_Text{
							Text: "pass",
						},
					}
					err = graph.Check(flow)
				)
				if !errors.Is(
					err,
					tc.err,
				) {
					t.Fatalf(
						"check: err = %v, want %v",
						err,
						tc.err,
					)
				}

				if tc.err != nil {
					_, err = graph.Emit(flow)
					if !errors.Is(
						err,
						tc.err,
					) {
						t.Fatalf(
							"emit: err = %v, want %v",
							err,
							tc.err,
						)
					}
				}
			},
		)
	}
}

// TestValue_JSONRefusesAnIntOutsideInt64 proves protojson refuses an int outside
// int64 and an int that is no decimal whole number, and reads the edges of the
// range, written as text or as a number (Q438, Q440). The rules are those of the
// protobuf JSON mapping for an int64 field. protojson reads an int written 1e3 or
// 1.0 as the int it is, as that mapping says.
//
// The check: go test -count=1 -run TestValue_JSONRefusesAnIntOutsideInt64
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 12:51: initial creation (C132, Q438, Q440)
func TestValue_JSONRefusesAnIntOutsideInt64(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want int64
		ok   bool
	}{
		{
			"zero",
			`{"int": "0"}`,
			0,
			true,
		},
		{
			"minus zero",
			`{"int": "-0"}`,
			0,
			true,
		},
		{
			"the largest",
			`{"int": "9223372036854775807"}`,
			math.MaxInt64,
			true,
		},
		{
			"the smallest",
			`{"int": "-9223372036854775808"}`,
			math.MinInt64,
			true,
		},
		{
			"a number",
			`{"int": 12}`,
			12,
			true,
		},
		{
			"an exponent",
			`{"int": 1e3}`,
			1000,
			true,
		},
		{
			"one past the largest",
			`{"int": "9223372036854775808"}`,
			0,
			false,
		},
		{
			"one past the smallest",
			`{"int": "-9223372036854775809"}`,
			0,
			false,
		},
		{
			"one past the largest as a number",
			`{"int": 9223372036854775808}`,
			0,
			false,
		},
		{
			"2^64+1",
			`{"int": "18446744073709551617"}`,
			0,
			false,
		},
		{
			"empty",
			`{"int": ""}`,
			0,
			false,
		},
		{
			"a fraction",
			`{"int": "1.5"}`,
			0,
			false,
		},
		{
			"a fraction as a number",
			`{"int": 1.5}`,
			0,
			false,
		},
		{
			"hex",
			`{"int": "0x10"}`,
			0,
			false,
		},
		{
			"a leading zero",
			`{"int": "01"}`,
			0,
			false,
		},
		{
			"a plus sign",
			`{"int": "+1"}`,
			0,
			false,
		},
		{
			"a space",
			`{"int": " 1"}`,
			0,
			false,
		},
		{
			"a line break",
			`{"int": "1\n"}`,
			0,
			false,
		},
		{
			"an underscore",
			`{"int": "1_000"}`,
			0,
			false,
		},
		{
			"Arabic-Indic digits",
			`{"int": "١٢"}`,
			0,
			false,
		},
		{
			"script text",
			`{"int": "1); import_os("}`,
			0,
			false,
		},
		{
			"a minus sign alone",
			`{"int": "-"}`,
			0,
			false,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					value = new(workflowpb.Value)
					err   = protojson.Unmarshal(
						[]byte(tc.json),
						value,
					)
				)
				if (err == nil) != tc.ok {
					t.Fatalf(
						"err = %v, want ok = %v",
						err,
						tc.ok,
					)
				}

				if tc.ok && value.GetInt() != tc.want {
					t.Fatalf(
						"got %d, want %d",
						value.GetInt(),
						tc.want,
					)
				}
			},
		)
	}
}

// TestLiteral_StatesNoValueForWhatHasNone proves a literal that go.starlark.net
// would not build, a typed nil int, a float that is NaN or infinite, a value of a
// type no literal has and a nil literal, states no value, and that a bytes literal
// is bytes and not a string (B290, Q407).
//
// The check: go test -count=1 -run TestLiteral_StatesNoValueForWhatHasNone
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:27: initial creation (C132, Q65, Q407, Q421)
func TestLiteral_StatesNoValueForWhatHasNone(t *testing.T) {
	var big70 = new(big.Int).Lsh(
		big.NewInt(1),
		70,
	)

	for _, tc := range []struct {
		name    string
		literal *syntax.Literal
		want    string
		stated  bool
	}{
		{
			"nil",
			nil,
			"",
			false,
		},
		{
			"typed nil int",
			&syntax.Literal{
				Token: syntax.INT,
				Value: (*big.Int)(nil),
			},
			"",
			false,
		},
		{
			"NaN",
			&syntax.Literal{
				Token: syntax.FLOAT,
				Value: math.NaN(),
			},
			"",
			false,
		},
		{
			"infinity",
			&syntax.Literal{
				Token: syntax.FLOAT,
				Value: math.Inf(1),
			},
			"",
			false,
		},
		{
			"no type",
			&syntax.Literal{
				Token: syntax.INT,
				Value: 1,
			},
			"",
			false,
		},
		{
			"string",
			&syntax.Literal{
				Token: syntax.STRING,
				Value: "a",
			},
			"string:a",
			true,
		},
		{
			"bytes",
			&syntax.Literal{
				Token: syntax.BYTES,
				Value: "a",
			},
			"bytes:a",
			true,
		},
		{
			"int",
			&syntax.Literal{
				Token: syntax.INT,
				Value: int64(-7),
			},
			"int:-7",
			true,
		},
		{
			"an int past int64",
			&syntax.Literal{
				Token: syntax.INT,
				Value: big70,
			},
			"",
			false,
		},
		{
			"the largest int64",
			&syntax.Literal{
				Token: syntax.INT,
				Value: int64(math.MaxInt64),
			},
			"int:9223372036854775807",
			true,
		},
		{
			"a string that is not UTF-8",
			&syntax.Literal{
				Token: syntax.STRING,
				Value: "a\xffb",
			},
			"",
			false,
		},
		{
			"bytes that are not UTF-8",
			&syntax.Literal{
				Token: syntax.BYTES,
				Value: "a\xffb",
			},
			"bytes:a\xffb",
			true,
		},
		{
			"float",
			&syntax.Literal{
				Token: syntax.FLOAT,
				Value: 1.5,
			},
			"float:1.5",
			true,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				value, ok := graph.Literal(tc.literal)
				if ok != tc.stated {
					t.Fatalf(
						"stated = %v, want %v",
						ok,
						tc.stated,
					)
				}

				if !ok {
					return
				}

				var got string

				switch kind := value.GetKind().(type) {
				case *workflowpb.Value_String_:
					got = "string:" + kind.String_

				case *workflowpb.Value_Bytes:
					got = "bytes:" + string(kind.Bytes)

				case *workflowpb.Value_Int:
					got = "int:" + strconv.FormatInt(
						kind.Int,
						10,
					)

				case *workflowpb.Value_Float:
					got = "float:" + strconv.FormatFloat(
						kind.Float,
						'g',
						-1,
						64,
					)
				}

				if got != tc.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestOf_CarriesADictWithAKeyThatIsNoStringAsText proves a dict whose key is a
// number, a bytes or a bare word is not a map of the flow, which has string keys
// only (Q423), and the statement is carried as the text of an init statement
// that the script writes back as it was (Q406). A tuple is still refused, and so
// is a dict with a key that is a name.
//
// The check: go test -count=1 -run TestOf_CarriesADictWithAKeyThatIsNoStringAsText
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q406, Q407, Q423)
func TestOf_CarriesADictWithAKeyThatIsNoStringAsText(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		text   string
		err    error
	}{
		{
			"a number key",
			"X = {1: \"x\"}\n",
			"X = {1: \"x\"}",
			nil,
		},
		{
			"a float key",
			"X = {1.5: \"x\"}\n",
			"X = {1.5: \"x\"}",
			nil,
		},
		{
			"a bytes key",
			"X = {b\"k\": 1}\n",
			"X = {b\"k\": 1}",
			nil,
		},
		{
			"a bare word key",
			"X = {True: 1}\n",
			"X = {True: 1}",
			nil,
		},
		{
			"a None key",
			"X = {None: [1, b\"v\"]}\n",
			"X = {None: [1, b\"v\"]}",
			nil,
		},
		{
			"a key inside a list",
			"X = [{1: 2}]\n",
			"X = [{1: 2}]",
			nil,
		},
		{
			"an empty string key",
			"X = {\"\": 1}\n",
			"",
			graph.ErrCarry,
		},
		{
			"an int past int64",
			"X = 9223372036854775808\n",
			"",
			graph.ErrCarry,
		},
		{
			"an int past int64 in a list",
			"X = [1, 123456789012345678901234567890]\n",
			"",
			graph.ErrCarry,
		},
		{
			"1 shifted 70 places",
			"X = 1 << 70\n",
			"",
			graph.ErrCarry,
		},
		{
			"a call argument",
			"def f(a):\n    return a\nX = f({1: 2})\n",
			"X = f({1: 2})",
			nil,
		},
		{
			"a list with a name",
			"k = 1\nX = [1, k]\n",
			"",
			graph.ErrCarry,
		},
		{
			"a tuple as the value of a dict",
			"X = {\"k\": (1, 2)}\n",
			"",
			graph.ErrCarry,
		},
		{
			"a list as a key",
			"X = {[1]: 2}\n",
			"",
			graph.ErrCarry,
		},
		{
			"a tuple",
			"X = (1, 2)\n",
			"",
			graph.ErrCarry,
		},
		{
			"a tuple of bytes",
			"X = (b\"a\", 1)\n",
			"",
			graph.ErrCarry,
		},
		{
			"a tuple as a value of a dict",
			"X = {1: (1, 2)}\n",
			"",
			graph.ErrCarry,
		},
		{
			"a key that is a name",
			"k = 1\nX = {k: 1}\n",
			"",
			graph.ErrCarry,
		},
		{
			"a key that is a tuple",
			"X = {(1, 2): 1}\n",
			"",
			graph.ErrCarry,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := graph.Of(&common.Source{
					Entry: "script.star",
					Text:  []byte(tc.script + VALUE_MAIN),
				})
				if tc.err != nil {
					if !errors.Is(
						err,
						tc.err,
					) {
						t.Fatalf(
							"derive: err = %v, want %v",
							err,
							tc.err,
						)
					}

					return
				}

				if err != nil {
					t.Fatalf(
						"derive: %v",
						err,
					)
				}

				if got := _Initial(flow); got != tc.text {
					t.Fatalf(
						"init text %q, want %q",
						got,
						tc.text,
					)
				}

				if len(flow.GetConstants()) != 0 {
					t.Fatalf(
						"the flow lists %v besides the init text",
						flow.GetConstants(),
					)
				}
			},
		)
	}
}

// TestOf_JudgesAnArgWithAKeyThatIsNoString proves an arg whose default is a dict with
// a key that is no string is carried as the text of an init statement only when it is
// a declaration of the builtin with a name and a default, and a malformed one is
// refused as it is for a default that is a string (Q406, Q434). A script that defines
// an arg of its own is read as it defines it: the call is a call constant, and its
// argument count is the function's.
//
// The check: go test -count=1 -run TestOf_JudgesAnArgWithAKeyThatIsNoString
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 12:52: initial creation (C132, Q406, Q434)
//   - 2026-11-13 23:55: says argument count, formerly arity (C136, Q475)
func TestOf_JudgesAnArgWithAKeyThatIsNoString(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		text   string
		err    error
	}{
		{
			"a declaration",
			"a = arg(\"a\", {1: 2})\ndef main():\n    print(a)\n",
			"a = arg(\"a\", {1: 2})",
			nil,
		},
		{
			"a declaration with a keyword default",
			"a = arg(\"a\", default={1: 2})\ndef main():\n    print(a)\n",
			"a = arg(\"a\", default={1: 2})",
			nil,
		},
		{
			"a declaration with three arguments",
			"a = arg(\"a\", {1: 2}, 3)\ndef main():\n    print(a)\n",
			"",
			graph.ErrCarry,
		},
		{
			"a declaration named by keyword",
			"a = arg(name=\"a\", default={1: 2})\ndef main():\n    print(a)\n",
			"",
			graph.ErrCarry,
		},
		{
			"a declaration with an empty name",
			"a = arg(\"\", {1: 2})\ndef main():\n    print(a)\n",
			"",
			graph.ErrCarry,
		},
		{
			"an arg the script defines, with two parameters",
			"def arg(n, d):\n    return d\nX = arg(\"a\", {1: 2})\n" + VALUE_MAIN,
			"X = arg(\"a\", {1: 2})",
			nil,
		},
		{
			"an arg the script defines, with one parameter, is a call as any other",
			"def arg(n):\n    return n\nX = arg(\"a\", {1: 2})\n" + VALUE_MAIN,
			"X = arg(\"a\", {1: 2})",
			nil,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := graph.Of(&common.Source{
					Entry: "script.star",
					Text:  []byte(tc.script),
				})
				if tc.err != nil {
					if !errors.Is(
						err,
						tc.err,
					) {
						t.Fatalf(
							"derive: err = %v, want %v",
							err,
							tc.err,
						)
					}

					return
				}

				if err != nil {
					t.Fatalf(
						"derive: %v",
						err,
					)
				}

				if got := _Initial(flow); got != tc.text {
					t.Fatalf(
						"init text %q, want %q",
						got,
						tc.text,
					)
				}

				if len(flow.GetArgs())+len(flow.GetConstants()) != 0 {
					t.Fatalf(
						"the flow lists %v and %v besides the init text",
						flow.GetArgs(),
						flow.GetConstants(),
					)
				}
			},
		)
	}
}

// TestCheck_RefusesAValueBuiltInGoThatIsNone proves a value a Go host builds, not
// one protojson read, is refused when it is a null kind that holds anything but null,
// a string that is not valid UTF-8, or a map whose key is not (Q436, Q439).
//
// The check: go test -count=1 -run TestCheck_RefusesAValueBuiltInGoThatIsNone
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 12:52: initial creation (C132, Q436, Q439)
func TestCheck_RefusesAValueBuiltInGoThatIsNone(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *workflowpb.Value
	}{
		{
			"a null that holds 5",
			&workflowpb.Value{
				Kind: &workflowpb.Value_Null{
					Null: 5,
				},
			},
		},
		{
			"a string that is not UTF-8",
			&workflowpb.Value{
				Kind: &workflowpb.Value_String_{
					String_: "a\xffb",
				},
			},
		},
		{
			"a map key that is not UTF-8",
			&workflowpb.Value{
				Kind: &workflowpb.Value_Map{
					Map: &workflowpb.Map{
						Entries: map[string]*workflowpb.Value{
							"a\xffb": {
								Kind: &workflowpb.Value_Bool{
									Bool: true,
								},
							},
						},
					},
				},
			},
		},
		{
			"a nil value",
			nil,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var flow = &workflowpb.Flow{
					Constants: map[string]*workflowpb.Constant{
						"X": {
							Kind: &workflowpb.Constant_Value{
								Value: tc.value,
							},
						},
					},
					Spine: &workflowpb.Flow_Text{
						Text: "pass",
					},
				}

				err := graph.Check(flow)
				if !errors.Is(
					err,
					graph.ErrForm,
				) {
					t.Fatalf(
						"check: err = %v, want %v",
						err,
						graph.ErrForm,
					)
				}

				_, err = graph.Emit(flow)
				if !errors.Is(
					err,
					graph.ErrForm,
				) {
					t.Fatalf(
						"emit: err = %v, want %v",
						err,
						graph.ErrForm,
					)
				}
			},
		)
	}
}

// TestOf_ReadsASourceByteThatIsNoUTF8AsTheReplacement proves a string constant never
// holds invalid UTF-8: go.starlark.net refuses the escapes \xff and \377 in a string,
// and reads a raw byte that is not UTF-8 as U+FFFD, so the value that convert carries is
// valid text. The check Literal makes for a string that is not valid UTF-8 (Q436) is a
// guard, tested through the exported Literal.
//
// The check: go test -count=1 -run TestOf_ReadsASourceByteThatIsNoUTF8AsTheReplacement
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 12:53: initial creation (C132, Q436, B294)
func TestOf_ReadsASourceByteThatIsNoUTF8AsTheReplacement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		err    bool
		want   string
	}{
		{
			"a raw byte",
			"X = \"a\xffb\"\n",
			false,
			"a\ufffdb",
		},
		{
			"a hex escape",
			"X = \"a\\xffb\"\n",
			true,
			"",
		},
		{
			"an octal escape",
			"X = \"a\\377b\"\n",
			true,
			"",
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := graph.Of(&common.Source{
					Entry: "script.star",
					Text:  []byte(tc.script + VALUE_MAIN),
				})
				if tc.err {
					if err == nil {
						t.Fatal("want an error")
					}

					return
				}

				if err != nil {
					t.Fatal(err)
				}

				got := flow.GetConstants()["X"].GetValue().GetString_()
				if got != tc.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}
