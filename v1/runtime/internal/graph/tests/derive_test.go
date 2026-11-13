package tests

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// DAG is the dag fixture: a script of five functions whose main spawns and
	// joins them in a dag.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/dag.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	DAG = `
def task_a(region):
    pass

def task_b():
    pass

def task_c():
    pass

def task_d(both):
    pass

def task_e():
    pass

def main():
    a = spawn(lambda: task_a("west"))
    b = spawn(task_b)
    join(a)
    c = spawn(task_c)
    got = join(a, b)
    d = spawn(lambda: task_d(got))
    join(b)
    e = spawn(task_e)
    join(c, d, e)
    pass
`

	// DAG_FLOW is the flow the spec writes for DAG, as JSON.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/dag.json (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
	DAG_FLOW = `
{
  "functions": [
    {"name": "task_a", "params": ["region"], "body": "pass"},
    {"name": "task_b", "body": "pass"},
    {"name": "task_c", "body": "pass"},
    {"name": "task_d", "params": ["both"], "body": "pass"},
    {"name": "task_e", "body": "pass"}
  ],
  "main": {
    "statement": [
      {"spawn": {"binding": "a", "call": {"function": "task_a", "args": [{"string": "west"}]}}},
      {"spawn": {"binding": "b", "call": {"function": "task_b"}}},
      {"join": {"bindings": ["a"]}},
      {"spawn": {"binding": "c", "call": {"function": "task_c"}}},
      {"join": {"bindings": ["a", "b"], "result": "got"}},
      {"spawn": {"binding": "d", "call": {"function": "task_d", "operands": [{"name": "got"}]}}},
      {"join": {"bindings": ["b"]}},
      {"spawn": {"binding": "e", "call": {"function": "task_e"}}},
      {"join": {"bindings": ["c", "d", "e"]}}
    ]
  }
}
`

	// EVERY is the every-construct fixture.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:25: holds the script text, inlined from testdata/every.star, and moved
	//     here
	//     from finished_test.go, which shares it with derive_test.go (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	EVERY = `
def sign(text, mark):
    pass

def task_a(region):
    sign(region, "!")
    spawn(work)
    pass

def task_b():
    pass

def task_c():
    pass

def task_d(both):
    pass

def task_e():
    pass

def ready():
    return True

def once():
    pass

def attempt(reason):
    pass

def slow():
    pass

def bounded():
    pass

def work():
    pass

def classify():
    return "alpha"
empty = None
flag = True
label = sign("hello", "!")
limit = 3
meta = {"k": 1}
ratio = 1.5
region = "west"
tags = ["a", "b"]
banner = sign(region, "!")
db = arg("host", "localhost")
optional = arg("optional", None)
port = arg("port")

def main():
    kept = sign("hello", "!")
    noted = sign(region, "!")
    h1 = spawn(lambda: task_a("west"))
    h2 = spawn(task_b)
    h3 = spawn(lambda: task_d(kept))
    h4 = spawn(task_c)
    h5 = spawn(task_e)
    join(h1)
    got = join(h1, h2)
    if ready():
        once()
        pass
    else:
        attempt("x")
        pass
    if True:
        bounded()
        pass
    else:
        join(h2)
        pass
    if False:
        slow()
        pass
    _match = classify()
    if _match == "alpha":
        bounded()
        pass
    elif _match == "beta":
        work()
        pass
    else:
        slow()
        pass
    _match = "gamma"
    if _match == "gamma":
        work()
        pass
    else:
        join(h1, h3)
        pass
    repeat(3, once)
    retry(2, lambda: attempt("x"))
    timeout(5000, slow)
    sleep(1000)
    cancel(h2, h4)
    for _ in range(limit):
        spawn(work)
        pass
    h6 = spawn(work)
    join(h5, h6)
    sign(empty, flag)
    sign(label, ratio)
    sign(tags, meta)
    sign(banner, db)
    sign(optional, port)
    pass
`

	// EVERY_FLOW is the flow the spec writes for EVERY, as JSON.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/every.json (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
	//nolint:lll // a line of the fixture text
	EVERY_FLOW = `
{
  "functions": [
    {"name": "sign", "params": ["text", "mark"], "body": "pass"},
    {
      "name": "task_a",
      "params": ["region"],
      "statements": {
        "statement": [
          {"call": {"function": "sign", "operands": [{"name": "region"}, {"literal": {"string": "!"}}]}},
          {"spawn": {"call": {"function": "work"}}}
        ]
      }
    },
    {"name": "task_b", "body": "pass"},
    {"name": "task_c", "body": "pass"},
    {"name": "task_d", "params": ["both"], "body": "pass"},
    {"name": "task_e", "body": "pass"},
    {"name": "ready", "body": "return True"},
    {"name": "once", "body": "pass"},
    {"name": "attempt", "params": ["reason"], "body": "pass"},
    {"name": "slow", "body": "pass"},
    {"name": "bounded", "body": "pass"},
    {"name": "work", "body": "pass"},
    {"name": "classify", "body": "return \"alpha\""}
  ],
  "constants": {
    "banner": {"call": {"function": "sign", "operands": [{"name": "region"}, {"literal": {"string": "!"}}]}},
    "empty": {"value": {"null": null}},
    "flag": {"value": {"bool": true}},
    "label": {"call": {"function": "sign", "args": [{"string": "hello"}, {"string": "!"}]}},
    "limit": {"value": {"int": "3"}},
    "meta": {"value": {"map": {"entries": {"k": {"int": "1"}}}}},
    "ratio": {"value": {"float": 1.5}},
    "region": {"value": {"string": "west"}},
    "tags": {"value": {"list": {"items": [{"string": "a"}, {"string": "b"}]}}}
  },
  "args": {
    "db": {"name": "host", "default": {"string": "localhost"}},
    "optional": {"name": "optional", "default": {"null": null}},
    "port": {"name": "port"}
  },
  "main": {
    "statement": [
      {"call": {"function": "sign", "args": [{"string": "hello"}, {"string": "!"}], "result": "kept"}},
      {"call": {"function": "sign", "operands": [{"name": "region"}, {"literal": {"string": "!"}}], "result": "noted"}},
      {"spawn": {"binding": "h1", "call": {"function": "task_a", "args": [{"string": "west"}]}}},
      {"spawn": {"binding": "h2", "call": {"function": "task_b"}}},
      {"spawn": {"binding": "h3", "call": {"function": "task_d", "operands": [{"name": "kept"}]}}},
      {"spawn": {"binding": "h4", "call": {"function": "task_c"}}},
      {"spawn": {"binding": "h5", "call": {"function": "task_e"}}},
      {"join": {"bindings": ["h1"]}},
      {"join": {"bindings": ["h1", "h2"], "result": "got"}},
      {
        "if": {
          "condition": {"call": {"function": "ready"}},
          "then": {"call": {"function": "once"}},
          "else": {"call": {"function": "attempt", "args": [{"string": "x"}]}}
        }
      },
      {
        "if": {
          "condition": {"value": true},
          "then": {"call": {"function": "bounded"}},
          "else": {"join": {"bindings": ["h2"]}}
        }
      },
      {
        "if": {
          "condition": {"value": false},
          "then": {"call": {"function": "slow"}}
        }
      },
      {
        "match": {
          "expression": {"call": {"function": "classify"}},
          "cases": [
            {"value": "alpha", "statement": {"call": {"function": "bounded"}}},
            {"value": "beta", "statement": {"call": {"function": "work"}}}
          ],
          "default": {"call": {"function": "slow"}}
        }
      },
      {
        "match": {
          "expression": {"value": "gamma"},
          "cases": [
            {"value": "gamma", "statement": {"call": {"function": "work"}}}
          ],
          "default": {"join": {"bindings": ["h1", "h3"]}}
        }
      },
      {"repeat": {"call": {"function": "once"}, "count": 3}},
      {"retry": {"call": {"function": "attempt", "args": [{"string": "x"}]}, "attempts": 2}},
      {"timeout": {"call": {"function": "slow"}, "timeoutMs": 5000}},
      {"sleep": {"durationMs": 1000}},
      {"cancel": {"bindings": ["h2", "h4"]}},
      {"loop": {"times": {"name": "limit"}, "body": {"spawn": {"call": {"function": "work"}}}}},
      {"spawn": {"binding": "h6", "call": {"function": "work"}}},
      {"join": {"bindings": ["h5", "h6"]}},
      {"call": {"function": "sign", "operands": [{"name": "empty"}, {"name": "flag"}]}},
      {"call": {"function": "sign", "operands": [{"name": "label"}, {"name": "ratio"}]}},
      {"call": {"function": "sign", "operands": [{"name": "tags"}, {"name": "meta"}]}},
      {"call": {"function": "sign", "operands": [{"name": "banner"}, {"name": "db"}]}},
      {"call": {"function": "sign", "operands": [{"name": "optional"}, {"name": "port"}]}}
    ]
  }
}
`

	// SHAPES is the flow the spec writes for the shapes fixture, as JSON. It has no
	// script.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/shapes.json (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-13 10:16: the values of its flows are tagged (C132, Q422)
	//nolint:lll // a line of the fixture text
	SHAPES = `
{
  "functions": [
    {"name": "work", "body": "pass"},
    {"name": "ready", "body": "return True"},
    {"name": "classify", "body": "return \"alpha\""},
    {"name": "slow", "body": "pass"},
    {"name": "inline", "body": "join(spawn(work), spawn(work))"},
    {"name": "bound", "body": "for _ in range(3):\n    h = spawn(work)\njoin(h)"},
    {"name": "over_list", "body": "for item in tags:\n    work()"},
    {"name": "escaped", "body": "if ready():\n    h = spawn(work)\njoin(h)"},
    {"name": "compared", "body": "if region == \"west\":\n    work()"},
    {"name": "watch", "params": ["h"], "body": "join(h)"},
    {
      "name": "passed",
      "statements": {
        "statement": [
          {"spawn": {"binding": "h", "call": {"function": "work"}}},
          {"spawn": {"binding": "w", "call": {"function": "watch", "operands": [{"name": "h"}]}}},
          {"join": {"bindings": ["w"]}}
        ]
      }
    },
    {"name": "other", "body": "kind = classify()\nif kind == \"alpha\":\n    work()\nelif kind == \"beta\":\n    slow()"},
    {
      "name": "opened",
      "statements": {
        "statement": [
          {"loop": {"times": {"name": "limit"}, "body": {"spawn": {"call": {"function": "work"}}}}}
        ]
      }
    }
  ],
  "constants": {
    "limit": {"value": {"int": "3"}},
    "region": {"value": {"string": "west"}},
    "tags": {"value": {"list": {"items": [{"string": "a"}, {"string": "b"}]}}}
  },
  "main": {
    "statement": [
      {"call": {"function": "inline"}},
      {"call": {"function": "bound"}},
      {"call": {"function": "over_list"}},
      {"call": {"function": "escaped"}},
      {"call": {"function": "compared"}},
      {"call": {"function": "passed"}},
      {"call": {"function": "other"}},
      {"call": {"function": "opened"}}
    ]
  }
}
`
)

// _Text is a script const's text without the newline that follows its opening
// backtick, which keeps the first line of the script off the line of the const.
//
// Revisions:
//   - 2026-11-09 11:37: initial creation (C114, Q289)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Text(text string) string {
	return strings.TrimPrefix(
		text,
		"\n",
	)
}

// _Fixture is one of the spec's fixtures: its name, the script it writes and
// the flow the spec writes for it. A fixture with no script has only a flow.
//
// Revisions:
//   - 2026-11-09 11:25: initial creation, replacing the files in testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Fixture struct {
	name string
	star string
	flow string
}

var (
	// FIXTURES are the spec's fixtures: dag, every and shapes.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, replacing the files in testdata (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FIXTURES = []_Fixture{
		{
			name: "dag",
			star: _Text(DAG),
			flow: _Text(DAG_FLOW),
		},
		{
			name: "every",
			star: _Text(EVERY),
			flow: _Text(EVERY_FLOW),
		},
		{
			name: "shapes",
			flow: _Text(SHAPES),
		},
	}
)

// TestOf_Fixtures derives the spec's two fixtures and compares each with the
// flow the spec writes for it.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-11-09 11:25: reads FIXTURES, not testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOf_Fixtures(t *testing.T) {
	for _, fixture := range FIXTURES {
		if fixture.star == "" {
			continue
		}

		t.Run(
			fixture.name,
			func(t *testing.T) {
				var (
					want = _Flow(
						t,
						fixture.flow,
					)
					got = _Derived(
						t,
						fixture.star,
					)
				)

				_Same(
					t,
					got,
					want,
				)
			},
		)
	}
}

// TestOf_Statements derives one function at a time and compares what it
// carries: its statements as JSON, or its body text.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-13 10:16: the values of its flows are tagged, and rows for sleep and loop counts
//     of other kinds and a None condition (C132, Q407, Q421, Q422)
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
				`{"literal": {"string": "!"}}]}}]`,
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
				`{"literal": {"string": "!"}}]}}]`,
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
			name:  "sleep of a whole float is milliseconds",
			def:   "def f():\n    sleep(1000.0)\n",
			state: `[{"sleep": {"durationMs": 1000}}]`,
		},
		{
			name: "sleep of a string is text",
			def:  "def f():\n    sleep(\"1\")\n",
			text: "sleep(\"1\")",
		},
		{
			name: "sleep of a name is text",
			def:  "def f(n):\n    sleep(n)\n",
			text: "sleep(n)",
		},
		{
			name: "sleep past an int32 is text",
			def:  "def f():\n    sleep(99999999999)\n",
			text: "sleep(99999999999)",
		},
		{
			name: "sleep of an int too big for a float is text",
			def: "def f():\n    sleep(" + strings.Repeat(
				"9",
				400,
			) + ")\n",
			text: "sleep(" + strings.Repeat(
				"9",
				400,
			) + ")",
		},
		{
			name: "a loop of a fraction is text",
			def:  "def f():\n    for _ in range(2.5):\n        once()\n",
			text: "for _ in range(2.5):\n    once()",
		},
		{
			name: "a condition that is None is text",
			def:  "def f():\n    if None:\n        once()\n",
			text: "if None:\n    once()",
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
			state: `[{"retry": {"call": {"function": "attempt", ` +
				`"args": [{"string": "x"}]}, ` +
				`"attempts": 2, "delayMs": 250}}]`,
		},
		{
			name:  "timeout is milliseconds",
			def:   "def f():\n    timeout(5000, slow)\n",
			state: `[{"timeout": {"call": {"function": "slow"}, "timeoutMs": 5000}}]`,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					flow = _Derived(
						t,
						SHARED+tc.def+"\ndef main():\n    f()\n",
					)
					fn = _Named(
						t,
						flow,
						"f",
					)
				)

				if tc.text != "" {
					if fn.GetBody() != tc.text {
						t.Fatalf(
							"got body %q, statements %v, want body %q",
							fn.GetBody(),
							fn.GetStatements(),
							tc.text,
						)
					}

					return
				}

				want := _Statements(
					t,
					tc.state,
				)

				_Same(
					t,
					fn.GetStatements(),
					want,
				)
			},
		)
	}
}

// TestOf_BodyText records what body text keeps: line comments above, between
// and after the statements, and no closer at any depth.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
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
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					flow = _Derived(
						t,
						tc.def+"\ndef main():\n    f()\n",
					)
					fn = _Named(
						t,
						flow,
						"f",
					)
				)

				if fn.GetBody() != tc.text {
					t.Fatalf(
						"got\n%q\nwant\n%q",
						fn.GetBody(),
						tc.text,
					)
				}
			},
		)
	}
}

// TestOf_Refuses records the scripts derivation will not carry at all.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrMain and ErrCarry, formerly ErrNoMain and ErrNotCarried (C121,
//     Q348)
func TestOf_Refuses(t *testing.T) {
	cases := []struct {
		name string
		src  string
		err  error
	}{
		{
			name: "no main",
			src:  "def f():\n    pass\n",
			err:  spelling.ErrMain,
		},
		{
			name: "a default",
			src:  "def f(x = 1):\n    pass\n\ndef main():\n    f()\n",
			err:  graph.ErrCarry,
		},
		{
			name: "a constant computed from an argument",
			src: "def f(x):\n    return x\n\nport = arg(\"port\")\nlabel = f(port)\n" +
				"\n" +
				"def main():\n    print(label)\n",
			err: graph.ErrCarry,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var source = &common.Source{
					Entry: "script.star",
					Text:  []byte(tc.src),
				}

				_, err := graph.Of(source)
				if !errors.Is(
					err,
					tc.err,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tc.err,
					)
				}
			},
		)
	}
}

// SHARED declares the functions, constants and argument the derive cases call.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	SHARED = `def once():
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
)

// _Derived is the flow a script derives to, failing the test on an error.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-10-03 00:16: the flow itself, which Of returns with nothing beside it
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Derived(
	t *testing.T,
	src string,
) *workflowpb.Flow {
	t.Helper()

	flow, err := graph.Of(&common.Source{
		Entry: "script.star",
		Text:  []byte(src),
	})
	if err != nil {
		t.Fatalf(
			"derive: %v",
			err,
		)
	}

	return flow
}

// _Named is the function a flow lists under name.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Named(
	t *testing.T,
	flow *workflowpb.Flow,
	name string,
) *workflowpb.Function {
	t.Helper()

	for _, fn := range flow.GetFunctions() {
		if fn.GetName() == name {
			return fn
		}
	}

	t.Fatalf(
		"no function %s",
		name,
	)

	return nil
}

// Flow is the flow a JSON text states.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Flow(
	t *testing.T,
	text string,
) *workflowpb.Flow {
	t.Helper()

	var (
		flow = new(workflowpb.Flow)
		err  error
	)

	err = protojson.Unmarshal(
		[]byte(text),
		flow,
	)
	if err != nil {
		t.Fatalf(
			"flow: %v",
			err,
		)
	}

	return flow
}

// _Statements is the statement list a JSON array states.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Statements(
	t *testing.T,
	text string,
) *workflowpb.Statements {
	t.Helper()

	var (
		held = new(workflowpb.Statements)
		err  error
	)

	err = protojson.Unmarshal(
		[]byte(`{"statement": `+text+`}`),
		held,
	)
	if err != nil {
		t.Fatalf(
			"statements: %v",
			err,
		)
	}

	return held
}

// _Same fails the test when two messages differ, printing both as JSON.
//
// Revisions:
//   - 2026-10-02 00:16: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Same(
	t *testing.T,
	got,
	want proto.Message,
) {
	t.Helper()

	if proto.Equal(
		got,
		want,
	) {
		return
	}

	shown := protojson.MarshalOptions{
		Multiline: true,
	}

	t.Fatalf(
		"got\n%s\nwant\n%s",
		shown.Format(got),
		shown.Format(want),
	)
}

// TestOf_RefusesNoSource proves Of answers no source at all with ErrMain, as
// the doc of Of says, and reads no field of it (Q65).
//
// The check: go test -count=1 -run TestOf_RefusesNoSource
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 00:04: initial creation (C130, Q405)
func TestOf_RefusesNoSource(t *testing.T) {
	_, err := graph.Of(nil)
	if !errors.Is(
		err,
		spelling.ErrMain,
	) {
		t.Fatalf(
			"err = %v, want ErrMain",
			err,
		)
	}
}
