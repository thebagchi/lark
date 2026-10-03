package graph_test

import (
	"errors"
	"os"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/graph"
	"github.com/thebagchi/lark/v1/runtime/script"
)

// TestOf_Fixtures derives the spec's two fixtures and compares each with the
// flow the spec writes for it.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func TestOf_Fixtures(t *testing.T) {
	for _, name := range []string{"dag", "every"} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + name + ".star")
			if err != nil {
				t.Fatal(err)
			}

			want := _Flow(t, _Read(t, "testdata/"+name+".json"))
			got := _Derived(t, string(src))

			_Same(t, got, want)
		})
	}
}

// TestOf_Statements derives one function at a time and compares what it
// carries: its statements as JSON, or its body text.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func TestOf_Statements(t *testing.T) {
	cases := []struct {
		name  string
		def   string
		state string
		text  string
	}{
		{
			name:  "a closer is not a statement",
			def:   "def f():\n    once()\n    pass\n",
			state: `[{"call": {"function": "once"}}]`,
		},
		{
			name: "a def that is only pass is text",
			def:  "def f():\n    pass\n",
			text: "pass",
		},
		{
			name: "a return is text",
			def:  "def f():\n    return 1\n",
			text: "return 1",
		},
		{
			name: "a library call is text",
			def:  "def f():\n    print(\"x\")\n",
			text: "print(\"x\")",
		},
		{
			name: "a parameter is an operand",
			def:  "def f(region):\n    sign(region, \"!\")\n",
			state: `[{"call": {"function": "sign", "operands": [{"name": "region"}, ` +
				`{"literal": "!"}]}}]`,
		},
		{
			name: "a parameter shadowing a function is text",
			def:  "def f(once):\n    once()\n",
			text: "once()",
		},
		{
			name: "a result is a name later on the list",
			def:  "def f():\n    kept = once()\n    sign(kept, \"!\")\n",
			state: `[{"call": {"function": "once", "result": "kept"}}, ` +
				`{"call": {"function": "sign", "operands": [{"name": "kept"}, ` +
				`{"literal": "!"}]}}]`,
		},
		{
			name: "join of a spawn written inside it is text",
			def:  "def f():\n    join(spawn(work), spawn(work))\n",
			text: "join(spawn(work), spawn(work))",
		},
		{
			name: "a loop whose body binds is text",
			def: "def f():\n    for _ in range(3):\n        h = spawn(work)\n" +
				"    join(h)\n",
			text: "for _ in range(3):\n    h = spawn(work)\njoin(h)",
		},
		{
			name: "a for over a list is text",
			def:  "def f():\n    for item in tags:\n        work()\n",
			text: "for item in tags:\n    work()",
		},
		{
			name: "a loop that reads its variable is text",
			def:  "def f():\n    for n in range(3):\n        sign(n, \"!\")\n",
			text: "for n in range(3):\n    sign(n, \"!\")",
		},
		{
			name: "a loop over range of a name",
			def: "def f():\n    for _ in range(limit):\n        spawn(work)\n" +
				"        pass\n",
			state: `[{"loop": {"times": {"name": "limit"}, ` +
				`"body": {"spawn": {"call": {"function": "work"}}}}}]`,
		},
		{
			name: "a binding made in a branch and used after it is text",
			def:  "def f():\n    if ready():\n        h = spawn(work)\n    join(h)\n",
			text: "if ready():\n    h = spawn(work)\njoin(h)",
		},
		{
			name: "a comparison is text",
			def:  "def f():\n    if region == \"west\":\n        work()\n",
			text: "if region == \"west\":\n    work()",
		},
		{
			name: "a join of a parameter is text",
			def:  "def f(h):\n    join(h)\n",
			text: "join(h)",
		},
		{
			name: "a handle passed as an argument",
			def: "def f():\n    h = spawn(work)\n    w = spawn(lambda: watch(h))\n" +
				"    join(w)\n",
			state: `[{"spawn": {"binding": "h", "call": {"function": "work"}}}, ` +
				`{"spawn": {"binding": "w", "call": {"function": "watch", ` +
				`"operands": [{"name": "h"}]}}}, ` +
				`{"join": {"bindings": ["w"]}}]`,
		},
		{
			name: "two spawns of one binding is text",
			def:  "def f():\n    h = spawn(work)\n    h = spawn(work)\n    join(h)\n",
			text: "h = spawn(work)\nh = spawn(work)\njoin(h)",
		},
		{
			name: "a chain on another name is text",
			def: "def f():\n    kind = classify()\n    if kind == \"alpha\":\n" +
				"        work()\n",
			text: "kind = classify()\nif kind == \"alpha\":\n    work()",
		},
		{
			name: "an elif is an if in the else",
			def: "def f():\n    if ready():\n        once()\n    elif flag:\n" +
				"        slow()\n",
			state: `[{"if": {"condition": {"call": {"function": "ready"}}, ` +
				`"then": {"call": {"function": "once"}}, ` +
				`"else": {"if": {"condition": {"name": "flag"}, ` +
				`"then": {"call": {"function": "slow"}}}}}}]`,
		},
		{
			name: "a negated condition is text",
			def:  "def f():\n    if not ready():\n        once()\n",
			text: "if not ready():\n    once()",
		},
		{
			name: "a match on a name",
			def: "def f():\n    _match = region\n    if _match == \"west\":\n" +
				"        once()\n    else:\n        slow()\n",
			state: `[{"match": {"expression": {"name": "region"}, ` +
				`"cases": [{"value": "west", ` +
				`"statement": {"call": {"function": "once"}}}], ` +
				`"default": {"call": {"function": "slow"}}}}]`,
		},
		{
			name:  "sleep is milliseconds",
			def:   "def f():\n    sleep(1000)\n",
			state: `[{"sleep": {"durationMs": 1000}}]`,
		},
		{
			name: "a fraction of a millisecond is text",
			def:  "def f():\n    sleep(0.5)\n",
			text: "sleep(0.5)",
		},
		{
			name: "a delay by position",
			def:  "def f():\n    repeat(3, once, 250)\n",
			state: `[{"repeat": {"call": {"function": "once"}, "count": 3, ` +
				`"delayMs": 250}}]`,
		},
		{
			name: "a delay by keyword",
			def:  "def f():\n    retry(2, lambda: attempt(\"x\"), delay = 250)\n",
			state: `[{"retry": {"call": {"function": "attempt", "args": ["x"]}, ` +
				`"attempts": 2, "delayMs": 250}}]`,
		},
		{
			name:  "timeout is milliseconds",
			def:   "def f():\n    timeout(5000, slow)\n",
			state: `[{"timeout": {"call": {"function": "slow"}, "timeoutMs": 5000}}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flow := _Derived(t, SHARED+tc.def+"\ndef main():\n    f()\n")
			fn := _Named(t, flow, "f")

			if tc.text != "" {
				if fn.GetBody() != tc.text {
					t.Fatalf("got body %q, statements %v, want body %q",
						fn.GetBody(), fn.GetStatements(), tc.text)
				}

				return
			}

			want := _Statements(t, tc.state)

			_Same(t, fn.GetStatements(), want)
		})
	}
}

// TestOf_BodyText records what body text keeps: line comments above, between
// and after the statements, and no closer at any depth.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func TestOf_BodyText(t *testing.T) {
	cases := []struct {
		name string
		def  string
		text string
	}{
		{
			name: "comments before, between and after",
			def: "def f():\n    # first\n    x = 1\n    # between\n    return x\n" +
				"    # last\n# outside\n",
			text: "# first\nx = 1\n# between\nreturn x\n# last",
		},
		{
			name: "closers at every depth",
			def: "def f():\n    if ready():\n        x = 1\n        pass\n" +
				"    for _ in range(3):\n        x = 2\n        pass\n    pass\n",
			text: "if ready():\n    x = 1\nfor _ in range(3):\n    x = 2",
		},
		{
			name: "a one-line def",
			def:  "def f(): return 1\n",
			text: "return 1",
		},
		{
			name: "an end-of-line comment is kept as written",
			def:  "def f():\n    x = 1  # one\n    return x\n",
			text: "x = 1  # one\nreturn x",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flow := _Derived(t, tc.def+"\ndef main():\n    f()\n")
			fn := _Named(t, flow, "f")

			if fn.GetBody() != tc.text {
				t.Fatalf("got\n%q\nwant\n%q", fn.GetBody(), tc.text)
			}
		})
	}
}

// TestOf_Refuses records the scripts derivation will not carry at all.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func TestOf_Refuses(t *testing.T) {
	cases := []struct {
		name string
		src  string
		err  error
	}{
		{name: "no main", src: "def f():\n    pass\n", err: graph.ERR_NO_MAIN},
		{
			name: "a default",
			src:  "def f(x = 1):\n    pass\n\ndef main():\n    f()\n",
			err:  graph.ERR_SIGNATURE,
		},
		{
			name: "a constant computed from an argument",
			src: "def f(x):\n    return x\n\nport = arg(\"port\")\nlabel = f(port)\n" +
				"\n" +
				"def main():\n    pass\n",
			err: graph.ERR_CONSTANT,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := &script.Source{Entry: "script.star", Text: []byte(tc.src)}

			_, err := graph.Of(source)
			if !errors.Is(err, tc.err) {
				t.Fatalf("got %v, want %v", err, tc.err)
			}
		})
	}
}

// SHARED declares the functions, constants and argument the derive cases call.
const SHARED = `def once():
    pass

def slow():
    pass

def work():
    pass

def ready():
    return True

def classify():
    return "alpha"

def sign(text, mark):
    pass

def attempt(reason):
    pass

def watch(h):
    join(h)

flag = True
limit = 3
region = "west"
tags = ["a", "b"]

`

// _Derived is the flow a script derives to, failing the test on an error.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-10-03 00:16: the flow itself, which Of returns with nothing beside it
func _Derived(t *testing.T, src string) *workflowpb.Flow {
	t.Helper()

	flow, err := graph.Of(&script.Source{Entry: "script.star", Text: []byte(src)})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	return flow
}

// _Named is the function a flow lists under name.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func _Named(t *testing.T, flow *workflowpb.Flow, name string) *workflowpb.Function {
	t.Helper()

	for _, fn := range flow.GetFunctions() {
		if fn.GetName() == name {
			return fn
		}
	}

	t.Fatalf("no function %s", name)

	return nil
}

// _Read is a file's text, failing the test when it cannot be read.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func _Read(t *testing.T, path string) string {
	t.Helper()

	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(text)
}

// _Flow is the flow a JSON text states.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func _Flow(t *testing.T, text string) *workflowpb.Flow {
	t.Helper()

	flow := new(workflowpb.Flow)

	err := protojson.Unmarshal([]byte(text), flow)
	if err != nil {
		t.Fatalf("flow: %v", err)
	}

	return flow
}

// _Statements is the statement list a JSON array states.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func _Statements(t *testing.T, text string) *workflowpb.Statements {
	t.Helper()

	held := new(workflowpb.Statements)

	err := protojson.Unmarshal([]byte(`{"statement": `+text+`}`), held)
	if err != nil {
		t.Fatalf("statements: %v", err)
	}

	return held
}

// _Same fails the test when two messages differ, printing both as JSON.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
func _Same(t *testing.T, got, want proto.Message) {
	t.Helper()

	if proto.Equal(got, want) {
		return
	}

	shown := protojson.MarshalOptions{Multiline: true}

	t.Fatalf("got\n%s\nwant\n%s", shown.Format(got), shown.Format(want))
}
