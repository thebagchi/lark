package tests

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// _Ran is how a script ended when it ran with args: the error text, or empty.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Ran(
	t *testing.T,
	src []byte,
	args map[string]any,
) string {
	t.Helper()

	art, err := runtime.Compile(&common.Source{
		Entry: "setup.star",
		Text:  src,
	})
	if err != nil {
		return "compile: " + err.Error()
	}

	run, err := runtime.Start(
		t.Context(),
		art,
		runtime.WithArgs(args),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err != nil {
		return _Unplaced(err.Error())
	}

	return ""
}

// _Generated is the flow a source derives, and the script that flow generates.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Generated(
	t *testing.T,
	src []byte,
	entry ...string,
) (*workflowpb.Flow, string) {
	t.Helper()

	var named = "setup.star"
	if len(entry) > 0 {
		named = entry[0]
	}

	flow, err := graph.Of(&common.Source{
		Entry: named,
		Text:  src,
	})
	if err != nil {
		t.Fatalf(
			"derive: %v",
			err,
		)
	}

	err = graph.Check(flow)
	if err != nil {
		t.Fatalf(
			"check: %v",
			err,
		)
	}

	out, err := graph.Emit(flow)
	if err != nil {
		t.Fatalf(
			"emit: %v",
			err,
		)
	}

	return flow, string(out)
}

// ASSERT is the top-level statement of samples/args.star.
//
// Revisions:
//   - 2026-11-05 00:01: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	ASSERT = `assert(retries > 0, "retries must be at least one")`
)

// _Held is what a flow's init holds, one string for each entry in order: the
// function of a call, "builtin" after it when the call is one, or the text.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-04 19:26: says whether a call is a builtin
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Held(flow *workflowpb.Flow) []string {
	var forms []string

	for _, held := range flow.GetInit() {
		call := held.GetCall()
		if call == nil {
			forms = append(
				forms,
				"text "+held.GetText(),
			)

			continue
		}

		word := "call " + call.GetFunction()
		if call.GetBuiltin() {
			word = "builtin " + call.GetFunction()
		}

		forms = append(
			forms,
			word,
		)
	}

	return forms
}

// TestRegression_B054_EntryStatementDropped proves the top-level assert of
// samples/args.star is in the flow as a call of the builtin, is generated
// again, and stops a run that supplies a bad argument, as the script does.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, bugs.md B054)
//   - 2026-11-04 19:26: the assert is a call of a builtin
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
func TestRegression_B054_EntryStatementDropped(t *testing.T) {
	src, err := os.ReadFile(SAMPLES + "args.star")
	if err != nil {
		t.Fatal(err)
	}

	flow, out := _Generated(
		t,
		src,
	)

	if got := strings.Join(
		_Held(flow),
		";",
	); got != "builtin assert" {
		t.Fatalf(
			"flow init %q, want one call of assert",
			got,
		)
	}

	call := flow.GetInit()[0].GetCall()

	if call.GetOperands()[0].GetExpression() != "retries > 0" ||
		call.GetOperands()[1].GetLiteral().GetString_() !=
			"retries must be at least one" {
		t.Fatalf(
			"assert carried as %v",
			call,
		)
	}

	if strings.Contains(
		out,
		"def init",
	) {
		t.Fatalf(
			"the script shows an init function\n%s",
			out,
		)
	}

	var (
		at = strings.Index(
			out,
			ASSERT,
		)
		binds = strings.Index(
			out,
			"retries = arg(",
		)
		defmain = strings.Index(
			out,
			"def main(",
		)
	)

	if at < 0 || binds < 0 || binds > at || at > defmain {
		t.Fatalf(
			"want the assert after the arguments and before main in\n%s",
			out,
		)
	}

	bad := map[string]any{"retries": 0}

	var (
		was = _Ran(
			t,
			src,
			bad,
		)
		now = _Ran(
			t,
			[]byte(out),
			bad,
		)
	)

	if !strings.Contains(
		was,
		"retries must be at least one",
	) {
		t.Fatalf(
			"the original did not stop: %q",
			was,
		)
	}

	if was != now {
		t.Fatalf(
			"ended differently\n was: %s\n now: %s",
			was,
			now,
		)
	}
}

// TestEntry_TopLevelStatementsAreCarried covers each kind of top-level
// statement an entry script holds: the flow carries it as a call, or as text
// when it is no call, and the generated script prints what the original
// printed.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-04 19:26: calls of builtins, keywords and expressions are calls
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEntry_TopLevelStatementsAreCarried(t *testing.T) {
	const (
		MAIN = "\ndef main():\n    print(\"main\")\n"
		WARM = "def warm(a, b):\n    print(\"warm\", a, b)\n\n"
	)

	tests := []struct {
		name  string
		src   string
		forms []string
	}{
		{
			name:  "print, positional",
			src:   "print(\"init\")\n" + MAIN,
			forms: []string{"builtin print"},
		},
		{
			name: "assert with an expression",
			src: "n = arg(\"n\", 1)\n\nassert(n > 0, \"n\")\n\n" +
				"def main():\n    print(n)\n",
			forms: []string{"builtin assert"},
		},
		{
			name:  "builtin with a keyword and an expression",
			src:   "print(\"%s:%d\" % (\"a\", 1), sep = \"-\")\n" + MAIN,
			forms: []string{"builtin print"},
		},
		{
			name:  "member of a predeclared name",
			src:   "print(hex.from_bytes(b\"A\"))\nhex.to_bytes(\"41\")\n" + MAIN,
			forms: []string{"builtin print", "builtin hex.to_bytes"},
		},
		{
			name:  "call of a declared function, positional",
			src:   WARM + "warm(\"x\", 2)\n" + MAIN,
			forms: []string{"call warm"},
		},
		{
			name:  "call of a declared function, keyword and positional",
			src:   WARM + "warm(\"x\", b=2)\n" + MAIN,
			forms: []string{"call warm"},
		},
		{
			name:  "unpacked arguments are text",
			src:   WARM + "args = [\"x\", 2]\nwarm(*args)\n" + MAIN,
			forms: []string{"text warm(*args)"},
		},
		{
			name:  "callee that is no name",
			src:   "[print][0](\"x\")\n" + MAIN,
			forms: []string{"text [print][0](\"x\")"},
		},
		{
			name:  "member of a value that is no name",
			src:   "\"a\".upper()\n" + MAIN,
			forms: []string{"text \"a\".upper()"},
		},
		{
			name:  "expression that is no call",
			src:   "\"doc\"\n" + MAIN,
			forms: []string{"text \"doc\""},
		},
		{
			name: "if",
			src: "n = arg(\"n\", 1)\n\nif n > 0:\n    print(\"pos\")\n" +
				"else:\n    print(\"neg\")\n\ndef main():\n    print(n)\n",
			forms: []string{
				"text if n > 0:\n    print(\"pos\")\nelse:\n    print(\"neg\")",
			},
		},
		{
			name: "for",
			src:  "for i in range(2):\n    print(\"i\", i)\n" + MAIN,
			forms: []string{
				"text for i in range(2):\n    print(\"i\", i)",
			},
		},
		{
			name: "augmented assignment",
			src: "items = [1]\nitems[0] += 2\nprint(\"items\", items)\n\n" +
				"def main():\n    print(items)\n",
			forms: []string{"text items[0] += 2", "builtin print"},
		},
		{
			name: "assignment that prints, then a statement reading it",
			src: "def noisy():\n    print(\"noisy\")\n    return 7\n\n" +
				"x = noisy()\nprint(\"x\", x)\n" + MAIN,
			forms: []string{"builtin print"},
		},
		{
			name: "statements keep their order",
			src: "def warm():\n    print(\"warm\")\n\n" +
				"print(\"one\")\nwarm()\nfor i in range(1):\n    print(i)\n" +
				"print(\"two\")\n" + MAIN,
			forms: []string{
				"builtin print",
				"call warm",
				"text for i in range(1):\n    print(i)",
				"builtin print",
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					flow, out = _Generated(
						t,
						[]byte(test.src),
					)
					forms = _Held(flow)
				)

				if strings.Join(
					forms,
					"|",
				) != strings.Join(
					test.forms,
					"|",
				) {
					t.Fatalf(
						"flow init %q, want %q",
						forms,
						test.forms,
					)
				}

				var (
					was = _Run(
						t,
						"setup.star",
						[]byte(test.src),
					)
					now = _Run(
						t,
						"setup.star",
						[]byte(out),
					)
				)

				if was.printed == "" {
					t.Fatal("the original printed nothing")
				}

				if was.printed != now.printed || was.failure != now.failure {
					t.Fatalf(
						"differs\n--- was\n%s\n--- now\n%s\n"+
							"--- generated\n%s",
						was.printed,
						now.printed,
						out,
					)
				}
			},
		)
	}
}

// _InitCall is a flow whose init calls function with the names args.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _InitCall(
	function string,
	args ...string,
) *workflowpb.Flow {
	var operands []*workflowpb.Operand

	for _, name := range args {
		operands = append(
			operands,
			&workflowpb.Operand{
				Source: &workflowpb.Operand_Name{
					Name: name,
				},
			},
		)
	}

	return &workflowpb.Flow{
		Functions: []*workflowpb.Function{{
			Name:   "warm",
			Params: []string{"a"},
			Code: &workflowpb.Function_Body{
				Body: "print(a)",
			},
		}},
		Init: []*workflowpb.Init{{
			Form: &workflowpb.Init_Call{
				Call: &workflowpb.Call{
					Function: function,
					Operands: operands,
				},
			},
		}},
		Spine: &workflowpb.Flow_Text{
			Text: "print(\"main\")",
		},
	}
}

// TestInit_CheckRefusesWhatMainsCallsAreRefusedFor proves a structured init
// call goes through the checks a call in main goes through.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestInit_CheckRefusesWhatMainsCallsAreRefusedFor(t *testing.T) {
	tests := []struct {
		name string
		flow *workflowpb.Flow
		ok   bool
	}{
		{
			"fits",
			_InitCall(
				"warm",
				"warm",
			),
			true,
		},
		{
			"name that resolves to nothing",
			_InitCall(
				"warm",
				"x",
			),
			false,
		},
		{
			"too many arguments",
			_InitCall(
				"warm",
				"warm",
				"warm",
			),
			false,
		},
		{
			"undeclared function",
			_InitCall("cold"),
			false,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := graph.Check(test.flow)
				if test.ok && err != nil {
					t.Fatalf(
						"want no refusal, got %v",
						err,
					)
				}

				if !test.ok && err == nil {
					t.Fatal("want a refusal")
				}
			},
		)
	}
}

// TestInit_EverySampleKeepsItsInit proves script to flow to script to flow
// gives one init, for every sample the derivation carries.
//
// The rest of a flow is not compared: a generated body drops blank lines, so
// a second derivation differs there, which the second-pass test of the
// scripts covers.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestInit_EverySampleKeepsItsInit(t *testing.T) {
	for _, name := range _Entries(t) {
		t.Run(
			name,
			func(t *testing.T) {
				if _Uncarried(name) {
					return
				}

				src, err := os.ReadFile(SAMPLES + name)
				if err != nil {
					t.Fatal(err)
				}

				var (
					once, out = _Generated(
						t,
						src,
						SAMPLES+name,
					)
					twice, _ = _Generated(
						t,
						[]byte(out),
						SAMPLES+name,
					)
				)

				if len(once.GetInit()) != len(twice.GetInit()) {
					t.Fatalf(
						"init %v then %v",
						once.GetInit(),
						twice.GetInit(),
					)
				}

				for idx := range once.GetInit() {
					if !proto.Equal(
						once.GetInit()[idx],
						twice.GetInit()[idx],
					) {
						t.Fatalf(
							"init %d: %v then %v",
							idx,
							once.GetInit()[idx],
							twice.GetInit()[idx],
						)
					}
				}
			},
		)
	}
}

// TestCall_BuiltinInMainAndInit proves a flow a host wrote, with a call of a
// builtin that has positional, keyword and expression arguments in init and in
// main, generates a script that runs.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
func TestCall_BuiltinInMainAndInit(t *testing.T) {
	var (
		expr = func(text string) *workflowpb.Operand {
			return &workflowpb.Operand{
				Source: &workflowpb.Operand_Expression{
					Expression: text,
				},
			}
		}
		literal = func(text string) *workflowpb.Operand {
			return &workflowpb.Operand{
				Source: &workflowpb.Operand_Literal{
					Literal: &workflowpb.Value{
						Kind: &workflowpb.Value_String_{
							String_: text,
						},
					},
				},
			}
		}
		show = func(function string) *workflowpb.Call {
			return &workflowpb.Call{
				Function: function,
				Builtin:  true,
				Operands: []*workflowpb.Operand{
					literal("%s:%d"),
					expr("1 + 1"),
				},
				Keywords: []*workflowpb.Keyword{{
					Name:  "sep",
					Value: literal("|"),
				}},
			}
		}
	)

	var flow = &workflowpb.Flow{
		Init: []*workflowpb.Init{{
			Form: &workflowpb.Init_Call{
				Call: show("print"),
			},
		}},
		Spine: &workflowpb.Flow_Main{
			Main: &workflowpb.Statements{
				Statement: []*workflowpb.Statement{{
					Action: &workflowpb.Statement_Call{
						Call: show("print"),
					},
				}},
			},
		},
	}

	var err = graph.Check(flow)
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Emit(flow)
	if err != nil {
		t.Fatal(err)
	}

	const WANT = "print(\"%s:%d\", 1 + 1, sep = \"|\")"

	if strings.Count(
		string(out),
		WANT,
	) != 2 {
		t.Fatalf(
			"want two of %s in\n%s",
			WANT,
			out,
		)
	}

	got := _Run(
		t,
		"setup.star",
		out,
	)
	if got.failure != "" || got.printed != "%s:%d|2\n%s:%d|2" &&
		got.printed != "%s:%d|2%s:%d|2" {
		t.Fatalf(
			"ran as %q, failed %q",
			got.printed,
			got.failure,
		)
	}
}

// TestCall_AnOldFlowStillLoadsAndRuns proves a flow written before keywords,
// expressions and builtin existed reads and runs the same.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-13 10:16: its old flow holds tagged args (C132, Q422)
func TestCall_AnOldFlowStillLoadsAndRuns(t *testing.T) {
	const OLD = `{
  "functions": [
    {"name": "greet", "params": ["who"], "body": "print(\"hello\", who)"}
  ],
  "main": {"statement": [
    {"call": {"function": "greet", "args": [{"string": "world"}]}}
  ]}
}`

	var flow workflowpb.Flow

	err := protojson.Unmarshal(
		[]byte(OLD),
		&flow,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = graph.Check(&flow)
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Emit(&flow)
	if err != nil {
		t.Fatal(err)
	}

	got := _Run(
		t,
		"setup.star",
		out,
	)
	if got.failure != "" || got.printed != "hello world" {
		t.Fatalf(
			"ran as %q, failed %q\n%s",
			got.printed,
			got.failure,
			out,
		)
	}

	call := flow.GetMain().GetStatement()[0].GetCall()
	if call.GetBuiltin() || len(call.GetKeywords()) != 0 {
		t.Fatalf(
			"an old call reads as %v",
			call,
		)
	}
}

// TestCall_CheckOfABuiltin says which checks a call of a builtin gets: both
// lists at once, a keyword with no name or one passed twice, an expression
// that is not one expression, a name an operand passes that resolves to
// nothing, and a builtin named as a function the flow declares are refused;
// the number of arguments and the name itself are not checked.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCall_CheckOfABuiltin(t *testing.T) {
	var (
		keyword = func(name string) *workflowpb.Keyword {
			return &workflowpb.Keyword{
				Name: name,
				Value: &workflowpb.Operand{
					Source: &workflowpb.Operand_Expression{
						Expression: "1",
					},
				},
			}
		}
		expr = func(text string) *workflowpb.Operand {
			return &workflowpb.Operand{
				Source: &workflowpb.Operand_Expression{
					Expression: text,
				},
			}
		}
		named = func(name string) *workflowpb.Operand {
			return &workflowpb.Operand{
				Source: &workflowpb.Operand_Name{
					Name: name,
				},
			}
		}
	)

	tests := []struct {
		name string
		call *workflowpb.Call
		ok   bool
	}{
		{
			"any count of arguments",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Operands: []*workflowpb.Operand{expr("1"), expr("2")},
			},
			true,
		},
		{
			"a name nobody declares",
			&workflowpb.Call{
				Function: "nosuch",
				Builtin:  true,
			},
			true,
		},
		{
			"keyword with no name",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Keywords: []*workflowpb.Keyword{keyword("")},
			},
			false,
		},
		{
			"keyword passed twice",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Keywords: []*workflowpb.Keyword{
					keyword("sep"),
					keyword("sep"),
				},
			},
			false,
		},
		{
			"expression that is not one",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Operands: []*workflowpb.Operand{expr("1 +")},
			},
			false,
		},
		{
			"expression that is a tuple without brackets",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Operands: []*workflowpb.Operand{expr("1, 2")},
			},
			false,
		},
		{
			"expression that ends in a comment",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Operands: []*workflowpb.Operand{expr("1 # c")},
			},
			false,
		},
		{
			"expression that is a keyword",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Operands: []*workflowpb.Operand{expr("sep = 1")},
			},
			false,
		},
		{
			"expression that unpacks",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Operands: []*workflowpb.Operand{expr("*items")},
			},
			false,
		},
		{
			"expression of several lines in a call",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Operands: []*workflowpb.Operand{expr("1 +\n  2")},
			},
			true,
		},
		{
			"name that resolves to nothing",
			&workflowpb.Call{
				Function: "print",
				Builtin:  true,
				Operands: []*workflowpb.Operand{named("ghost")},
			},
			false,
		},
		{
			"builtin named as a declared function",
			&workflowpb.Call{
				Function: "warm",
				Builtin:  true,
			},
			false,
		},
		{
			"declared function with a keyword counts it",
			&workflowpb.Call{
				Function: "warm",
				Keywords: []*workflowpb.Keyword{keyword("a")},
			},
			true,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var flow = &workflowpb.Flow{
					Functions: []*workflowpb.Function{{
						Name:   "warm",
						Params: []string{"a"},
						Code: &workflowpb.Function_Body{
							Body: "print(a)",
						},
					}},
					Init: []*workflowpb.Init{{
						Form: &workflowpb.Init_Call{
							Call: test.call,
						},
					}},
					Spine: &workflowpb.Flow_Text{
						Text: "print(\"main\")",
					},
				}

				var err = graph.Check(flow)
				if test.ok && err != nil {
					t.Fatalf(
						"want no refusal, got %v",
						err,
					)
				}

				if !test.ok && err == nil {
					t.Fatal("want a refusal")
				}
			},
		)
	}
}

// TestEntry_ExpressionArguments proves an argument that is an expression
// either round-trips as the script wrote it, line breaks and comments inside
// it included, or the statement is carried as text.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEntry_ExpressionArguments(t *testing.T) {
	const (
		MAIN = "\ndef main():\n    print(\"main\")\n"
		ARG  = "x = arg(\"x\", 3)\n"
	)

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "binary expression over two lines",
			src:  ARG + "print(\"a\", x >\n  1)\n" + MAIN,
			want: "x >\n  1",
		},
		{
			name: "boolean expression over two lines in an assert",
			src:  ARG + "assert(x > 1 and\n x < 9, \"m\")\n" + MAIN,
			want: "x > 1 and\n x < 9",
		},
		{
			name: "comment inside an expression",
			src:  ARG + "print(\"a\", x > # note\n 1)\n" + MAIN,
			want: "x > # note\n 1",
		},
		{
			name: "comment after an argument stays outside it",
			src:  ARG + "print(x + 1 # note\n)\n" + MAIN,
			want: "x + 1",
		},
		{
			name: "tuple in brackets",
			src:  "print((1, 2), 3)\n" + MAIN,
			want: "(1, 2)",
		},
		{
			name: "parenthesized call over several lines",
			src:  "print(max(\n  1,\n  2))\n" + MAIN,
			want: "max(\n  1,\n  2)",
		},
		{
			name: "keyword value over two lines",
			src:  ARG + "print(\"a\", \"b\", sep = str(x >\n  1))\n" + MAIN,
			want: "str(x >\n  1)",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					flow, out = _Generated(
						t,
						[]byte(test.src),
					)
					again, _ = _Generated(
						t,
						[]byte(out),
					)
					forms = _Held(flow)
				)

				if len(forms) != 1 || !strings.HasPrefix(
					forms[0],
					"builtin",
				) {
					t.Fatalf(
						"flow init %q, want one builtin call",
						forms,
					)
				}

				if !strings.Contains(
					out,
					test.want,
				) {
					t.Fatalf(
						"generated script lacks %q\n%s",
						test.want,
						out,
					)
				}

				if !proto.Equal(
					flow.GetInit()[0],
					again.GetInit()[0],
				) {
					t.Fatalf(
						"second derivation differs\n%v\n%v",
						flow.GetInit()[0],
						again.GetInit()[0],
					)
				}

				var (
					was = _Run(
						t,
						"setup.star",
						[]byte(test.src),
					)
					now = _Run(
						t,
						"setup.star",
						[]byte(out),
					)
				)

				if was.printed == "" {
					t.Fatal("the original printed nothing")
				}

				if was.printed != now.printed || was.failure != now.failure {
					t.Fatalf(
						"differs\n--- was\n%s\n--- now\n%s\n"+
							"--- generated\n%s",
						was.printed,
						now.printed,
						out,
					)
				}
			},
		)
	}
}

// TestInit_RefusalsOfEmitAndCheck covers what Emit and Check refuse in an
// init: an entry with no form, a keyword with no value, an expression that is
// not one argument, an empty entry, and a dotted builtin of three names.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestInit_RefusalsOfEmitAndCheck(t *testing.T) {
	var (
		builtin = func(call *workflowpb.Call) *workflowpb.Flow {
			call.Builtin = true

			return &workflowpb.Flow{
				Init: []*workflowpb.Init{{
					Form: &workflowpb.Init_Call{
						Call: call,
					},
				}},
				Spine: &workflowpb.Flow_Text{
					Text: "print(\"main\")",
				},
			}
		}
		empty = &workflowpb.Flow{
			Init: []*workflowpb.Init{{}},
			Spine: &workflowpb.Flow_Text{
				Text: "print(\"main\")",
			},
		}
		notes = &workflowpb.Flow{
			Init: []*workflowpb.Init{{
				Form: &workflowpb.Init_Text{
					Text: "# only a note",
				},
			}},
			Spine: &workflowpb.Flow_Text{
				Text: "print(\"main\")",
			},
		}
		bare = &workflowpb.Flow{
			Init: []*workflowpb.Init{{
				Form: &workflowpb.Init_Call{},
			}},
			Spine: &workflowpb.Flow_Text{
				Text: "print(\"main\")",
			},
		}
	)

	tests := []struct {
		name string
		flow *workflowpb.Flow
		want error
	}{
		{
			"entry with no form",
			empty,
			graph.ErrForm,
		},
		{
			"operand with no name",
			builtin(&workflowpb.Call{
				Function: "print",
				Operands: []*workflowpb.Operand{{
					Source: &workflowpb.Operand_Name{},
				}},
			}),
			nil,
		},
		{
			"operand with no source",
			builtin(&workflowpb.Call{
				Function: "print",
				Operands: []*workflowpb.Operand{{}},
			}),
			nil,
		},
		{
			"expression that is two arguments",
			builtin(&workflowpb.Call{
				Function: "print",
				Operands: []*workflowpb.Operand{{
					Source: &workflowpb.Operand_Expression{
						Expression: "1, 2",
					},
				}},
			}),
			graph.ErrForm,
		},
		{
			"keyword with no name",
			builtin(&workflowpb.Call{
				Function: "print",
				Keywords: []*workflowpb.Keyword{{
					Value: &workflowpb.Operand{
						Source: &workflowpb.Operand_Name{
							Name: "x",
						},
					},
				}},
			}),
			graph.ErrForm,
		},
		{
			"entry that is only a note",
			notes,
			graph.ErrForm,
		},
		{
			"entry with a nil call",
			bare,
			graph.ErrForm,
		},
		{
			"keyword with no value",
			builtin(&workflowpb.Call{
				Function: "print",
				Keywords: []*workflowpb.Keyword{{
					Name: "sep",
				}},
			}),
			nil,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := graph.Check(test.flow)
				if test.want != nil && !errors.Is(
					err,
					test.want,
				) {
					t.Fatalf(
						"check: got %v, want %v",
						err,
						test.want,
					)
				}

				_, err = graph.Emit(test.flow)
				if err == nil {
					t.Fatal("emit: want a refusal")
				}
			},
		)
	}
}

// TestInit_DottedBuiltinOfThreeNames proves a member of a member of a
// predeclared name is carried as a builtin call with the whole path.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestInit_DottedBuiltinOfThreeNames(t *testing.T) {
	var (
		flow, out = _Generated(
			t,
			[]byte("a.b.c(\"x\")\n\ndef main():\n    print(\"main\")\n"),
		)
		forms = _Held(flow)
	)

	if strings.Join(
		forms,
		"|",
	) != "builtin a.b.c" {
		t.Fatalf(
			"flow init %q",
			forms,
		)
	}

	if !strings.Contains(
		out,
		"a.b.c(\"x\")",
	) {
		t.Fatalf(
			"generated script lacks the call\n%s",
			out,
		)
	}
}

// TestRegression_B054_ModuleAssignmentStillRefused proves a loaded module's
// plain assignment is a constant and its augmented assignment is refused, now
// that the entry's is carried.
//
// Revisions:
//   - 2026-11-03 23:54: initial creation (C44c, B054)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func TestRegression_B054_ModuleAssignmentStillRefused(t *testing.T) {
	tests := []struct {
		name   string
		module string
		want   error
	}{
		{
			"plain assignment is a constant",
			"base = 1\n\ndef b():\n    return \"b\"\n",
			nil,
		},
		{
			"augmented assignment is refused",
			"base = 1\nbase += 1\n\ndef b():\n    return \"b\"\n",
			graph.ErrCarry,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := graph.Of(&common.Source{
					Entry: WALK_ENTRY,
					Text:  []byte("load(\"base.star\", \"b\")\n" + WALK_MAIN),
					Loader: &_Map{
						files: map[string][]byte{
							"base.star": []byte(test.module),
						},
					},
				})
				if test.want == nil && err != nil {
					t.Fatalf(
						"want no refusal, got %v",
						err,
					)
				}

				if test.want != nil && !errors.Is(
					err,
					test.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						test.want,
					)
				}
			},
		)
	}
}

// _Printing is a flow whose init calls print with one expression.
//
// Revisions:
//   - 2026-11-04 06:25: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Printing(text string) *workflowpb.Flow {
	return &workflowpb.Flow{
		Init: []*workflowpb.Init{{
			Form: &workflowpb.Init_Call{
				Call: &workflowpb.Call{
					Function: "print",
					Builtin:  true,
					Operands: []*workflowpb.Operand{{
						Source: &workflowpb.Operand_Expression{
							Expression: text,
						},
					}},
				},
			},
		}},
		Spine: &workflowpb.Flow_Text{
			Text: "print(0)",
		},
	}
}

// TestCall_ExpressionThatBreaksOutOfItsCall proves a text that closes the call
// it is written in, or opens a bracket it does not close, is refused by Check
// and by Emit, and a text that stands alone is accepted.
//
// Revisions:
//   - 2026-11-04 06:25: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCall_ExpressionThatBreaksOutOfItsCall(t *testing.T) {
	refused := []string{
		"1)(2",
		"a)(b",
		"x])[y",
		")",
		"(",
		")(",
		"1) + (2",
		"1), (2",
		"a).b(c",
		"1) if (2",
		"1)\n_(2",
		"1) # (",
		"1)\n(2",
		"1))",
		"((1)",
		"1]",
		"[1",
		"}",
		"{1",
		"1 # c",
		"1, 2",
		"*a",
		"**a",
		"a = 1",
		"",
		" ",
		"\"abc",
		"1 +",
		"lambda: 1)(",
		"1)\n\nprint(2",
	}

	for _, text := range refused {
		t.Run(
			"refuses "+strconv.Quote(text),
			func(t *testing.T) {
				flow := _Printing(text)

				if graph.Check(flow) == nil {
					t.Fatal("check: want a refusal")
				}

				_, err := graph.Emit(flow)
				if err == nil {
					t.Fatal("emit: want a refusal")
				}
			},
		)
	}

	accepted := []string{
		"1",
		"x",
		"x > 1",
		"x >\n  1",
		"x > # note\n 1",
		"1 #\n",
		"(1, 2)",
		"[1, 2]",
		"{\"a\": 1}",
		"f(\n  1,\n  2)",
		"lambda: 1",
		"\"a)(b\"",
		"\"\"\"1)(2\"\"\"",
		"x[1]",
		"-x",
	}

	for _, text := range accepted {
		t.Run(
			"accepts "+strconv.Quote(text),
			func(t *testing.T) {
				var (
					flow = _Printing(text)
					err  = graph.Check(flow)
				)
				if err != nil {
					t.Fatalf(
						"check: %v",
						err,
					)
				}

				_, err = graph.Emit(flow)
				if err != nil {
					t.Fatalf(
						"emit: %v",
						err,
					)
				}
			},
		)
	}
}

// FuzzExpressionArgument checks, for any text, that when Check accepts it as
// an expression argument, Emit writes a script that parses, whose first
// statement is one call of print with exactly one argument, and that the text
// is the same between the brackets when the script is read again.
//
// Revisions:
//   - 2026-11-04 06:25: initial creation (C44c, B054)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func FuzzExpressionArgument(f *testing.F) {
	for _, seed := range []string{
		"1", "x > 1", "x >\n  1", "1)(2", "a)(b", "x])[y", ")", "(",
		"1 # c", "1, 2", "(1, 2)", "f(\n1,\n2)", "\"a)(b\"", "1 #\n",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		flow := _Printing(text)

		if graph.Check(flow) != nil {
			return
		}

		out, err := graph.Emit(flow)
		if err != nil {
			t.Fatalf(
				"check accepted %q and emit refused: %v",
				text,
				err,
			)
		}

		file, err := common.OPTIONS.Parse(
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

		call := _FirstCall(file)
		if call == nil || len(call.Args) != 1 {
			t.Fatalf(
				"check accepted %q and the first statement is not print "+
					"with one argument\n%s",
				text,
				out,
			)
		}

		again, err := graph.Emit(_Printing(_Between(
			t,
			out,
		)))
		if err != nil {
			t.Fatalf(
				"the text between the brackets of %q is not stable: %v",
				text,
				err,
			)
		}

		if string(again) != string(out) {
			t.Fatalf(
				"emit of %q is not stable\n--- first\n%s\n--- second\n%s",
				text,
				out,
				again,
			)
		}
	})
}

// _FirstCall is the call a file's first statement is, or nil.
//
// Revisions:
//   - 2026-11-04 06:25: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _FirstCall(file *syntax.File) *syntax.CallExpr {
	for _, stmt := range file.Stmts {
		expr, ok := stmt.(*syntax.ExprStmt)
		if !ok {
			continue
		}

		call, ok := expr.X.(*syntax.CallExpr)
		if !ok {
			return nil
		}

		fn, ok := call.Fn.(*syntax.Ident)
		if !ok || fn.Name != "print" {
			return nil
		}

		return call
	}

	return nil
}

// _Between is the text between the brackets of the first print( in a script,
// up to the bracket that closes the line before the next statement.
//
// Revisions:
//   - 2026-11-04 06:25: initial creation (C44c, B054)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Between(
	t *testing.T,
	out []byte,
) string {
	t.Helper()

	const (
		OPEN  = "print("
		CLOSE = ")\n\ndef main"
	)

	var (
		text = string(out)
		from = strings.Index(
			text,
			OPEN,
		)
		to = strings.Index(
			text,
			CLOSE,
		)
	)

	if from < 0 || to < from {
		t.Fatalf(
			"no print call in\n%s",
			out,
		)
	}

	return text[from+len(OPEN) : to]
}
