package graph_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

// _Case is one flow and the script it writes.
type _Case struct {
	name string
	flow string
	want string
}

// _Refusal is one flow and the sentinel it returns.
type _Refusal struct {
	name string
	flow string
	err  error
}

// _Bodied is a flow whose one function f has body as its text, and whose main
// calls it.
//
// Revisions:
//   - 2026-10-02 00:18: initial creation
func _Bodied(t *testing.T, body string) string {
	t.Helper()

	quoted, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	return `{"functions": [{"name": "f", "body": ` + string(quoted) + `}],` +
		`"main": {"statement": [{"call": {"function": "f"}}]}}`
}

// MAIN is the main every body case ends with.
const MAIN = "\ndef main():\n    f()\n    pass\n"

// TestEmit_Writes is the script each flow writes.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 12:55: a one-line suite is rewritten onto its own line, and a
//     trailing comment stays on its statement
//   - 2026-10-02 00:18: lifted into graph as Emit's test: a trailing comment is
//     trimmed, a line comment stays where it was written, and durations are
//     milliseconds
//   - 2026-10-02 23:54: a statement ending in an index or a slice keeps its
//     closing bracket
func TestEmit_Writes(t *testing.T) {
	t.Parallel()

	cases := []_Case{
		{
			name: "return",
			flow: `{
				"functions": [{"name": "ready", "body": "return True"}],
				"main": {"statement": [{"call": {"function": "ready"}}]}
			}`,
			want: "def ready():\n    return True\n\ndef main():\n    ready()\n" +
				"    pass\n",
		},
		{
			name: "pass",
			flow: `{
				"functions": [{"name": "work", "body": "pass"}],
				"main": {"statement": [{"call": {"function": "work"}}]}
			}`,
			want: "def work():\n    pass\n\ndef main():\n    work()\n    pass\n",
		},
		{
			name: "if",
			flow: `{
				"functions": [{
					"name": "f",
					"statements": {"statement": [{
						"if": {
							"condition": {"value": true},
							"then": {"call": {"function": "once"}}
						}
					}]}
				}],
				"main": {"statement": [{"call": {"function": "f"}}]}
			}`,
			want: "def f():\n    if True:\n        once()\n        pass\n    pass\n" +
				"" + MAIN,
		},
		{
			name: "elif",
			flow: _Bodied(t, "if kind == \"alpha\":\n    work()\n"+
				"elif kind == \"beta\":\n    slow()"),
			want: "def f():\n    if kind == \"alpha\":\n        work()\n" +
				"        pass\n" +
				"    elif kind == \"beta\":\n        slow()\n        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "else if",
			flow: _Bodied(t, "if ready():\n    work()\nelse:\n    if flag:\n"+
				"        slow()"),
			want: "def f():\n    if ready():\n        work()\n        pass\n" +
				"    else:\n" +
				"        if flag:\n            slow()\n            pass\n" +
				"        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "while",
			flow: _Bodied(t, "while ready():\n    work()"),
			want: "def f():\n    while ready():\n        work()\n        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "break",
			flow: _Bodied(t, "for _ in range(3):\n    break"),
			want: "def f():\n    for _ in range(3):\n        break\n        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "durations",
			flow: `{
				"functions": [{"name": "once", "body": "pass"}],
				"main": {"statement": [
					{"repeat": {
						"call": {"function": "once"},
						"count": 3, "delayMs": 500}},
					{"timeout": {
						"call": {"function": "once"}, "timeoutMs": 5000}},
					{"sleep": {"durationMs": 1000}}
				]}
			}`,
			want: "def once():\n    pass\n\ndef main():\n    repeat(3, once, 500)\n" +
				"    timeout(5000, once)\n    sleep(1000)\n    pass\n",
		},
		{
			name: "constants",
			flow: `{
				"constants": {
					"banner": {"call": {"function": "sign", "operands": [
						{"name": "region"}, {"literal": "!"}
					]}},
					"region": {"value": "west"},
					"tags": {"value": ["a", "b"]},
					"label": {"call": {
						"function": "sign", "args": ["hello", "!"]}}
				},
				"main": {"statement": [{"call": {"function": "sign"}}]}
			}`,
			want: "label = sign(\"hello\", \"!\")\nregion = \"west\"\n" +
				"tags = [\"a\", \"b\"]\n" +
				"banner = sign(region, \"!\")\n\ndef main():\n    sign()\n" +
				"    pass\n",
		},
		{
			name: "one line",
			flow: _Bodied(t, "if ready(): once()"),
			want: "def f():\n    if ready():\n        once()\n        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "one line for",
			flow: _Bodied(t, "for _ in range(3): once()"),
			want: "def f():\n    for _ in range(3):\n        once()\n        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "inner line",
			flow: _Bodied(t, "if ready():\n    if flag: once()"),
			want: "def f():\n    if ready():\n        if flag:\n            once()\n" +
				"            pass\n        pass\n    pass\n" + MAIN,
		},
		{
			name: "one line def",
			flow: _Bodied(t, "def inner(): once()"),
			want: "def f():\n\n    def inner():\n        once()\n        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "comment on def",
			flow: _Bodied(t, "once()\n# helper\ndef inner():\n    twice()"),
			want: "def f():\n    once()\n\n    # helper\n    def inner():\n" +
				"        twice()\n" +
				"        pass\n    pass\n" + MAIN,
		},
		{
			name: "trailing comment",
			flow: _Bodied(t, "once()  # note"),
			want: "def f():\n    once()\n    pass\n" + MAIN,
		},
		{
			name: "trailing comment on a one-line suite",
			flow: _Bodied(t, "if ready(): once()  # note"),
			want: "def f():\n    if ready():\n        once()\n        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "trailing comment on a header",
			flow: _Bodied(t, "if ready():  # why\n    once()\nelse:  # other\n"+
				"    slow()"),
			want: "def f():\n    if ready():\n        once()\n        pass\n" +
				"    else:\n" +
				"        slow()\n        pass\n    pass\n" + MAIN,
		},
		{
			name: "line comments",
			flow: _Bodied(t, "# leading comment\nonce()\n\n# between\nonce()\n"+
				"# trailing comment"),
			want: "def f():\n    # leading comment\n    once()\n    # between\n" +
				"    once()\n" +
				"    # trailing comment\n    pass\n" + MAIN,
		},
		{
			name: "a comment at the end of a block",
			flow: _Bodied(t, "if ready():\n    # arm\n    once()\n    # after"),
			want: "def f():\n    if ready():\n        # arm\n        once()\n" +
				"        # after\n" +
				"        pass\n    pass\n" + MAIN,
		},
		{
			name: "a comment at the end of an inner block",
			flow: _Bodied(t, "if ready():\n    once()\n    # end of the if\nslow()"),
			want: "def f():\n    if ready():\n        once()\n" +
				"        # end of the if\n" +
				"        pass\n    slow()\n    pass\n" + MAIN,
		},
		{
			name: "a comment above an elif",
			flow: _Bodied(t, "if ready():\n    once()\n# about the elif\n"+
				"elif flag:\n    slow()"),
			want: "def f():\n    if ready():\n        once()\n        pass\n" +
				"    # about the elif\n    elif flag:\n        slow()\n" +
				"        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "a comment above an else",
			flow: _Bodied(t, "if ready():\n    once()\n# about the else\nelse:\n"+
				"    slow()"),
			want: "def f():\n    if ready():\n        once()\n        pass\n" +
				"    # about the else\n    else:\n        slow()\n        pass\n" +
				"    pass\n" + MAIN,
		},
		{
			name: "a comment after a final return",
			flow: _Bodied(t, "if done():\n    return 1\n    # log this later\nonce()"),
			want: "def f():\n    if done():\n        # log this later\n" +
				"        return 1\n" +
				"    once()\n    pass\n" + MAIN,
		},
		{
			name: "comments inside a list",
			flow: _Bodied(t, "x = [\n    1,  # first\n    # second follows\n    2,\n"+
				"]\nonce()"),
			want: "def f():\n    x = [\n        1,\n        # second follows\n" +
				"        2,\n" +
				"    ]\n    once()\n    pass\n" + MAIN,
		},
		{
			name: "an index at the end",
			flow: _Bodied(t, "got = [1, 2]\nreturn got[0] + got[1]"),
			want: "def f():\n    got = [1, 2]\n    return got[0] + got[1]\n" + MAIN,
		},
		{
			name: "a slice at the end",
			flow: _Bodied(t, "xs = [1, 2, 3]\nreturn xs[1:2]"),
			want: "def f():\n    xs = [1, 2, 3]\n    return xs[1:2]\n" + MAIN,
		},
		{
			name: "an index ending a statement",
			flow: _Bodied(t, "x = got[0]\nonce()"),
			want: "def f():\n    x = got[0]\n    once()\n    pass\n" + MAIN,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := graph.Emit(_Flow(t, tc.flow))
			if err != nil {
				t.Fatalf("emit: %v", err)
			}

			_, err = dialect.OPTIONS.Parse("out.star", got, 0)
			if err != nil {
				t.Fatalf("parse: %v\n%s", err, got)
			}

			if string(got) != tc.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// TestEmit_Refuses is the flow each sentinel rejects.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-02 00:18: lifted into graph as Emit's test
func TestEmit_Refuses(t *testing.T) {
	t.Parallel()

	cases := []_Refusal{
		{name: "empty", flow: `{}`, err: graph.ERR_NO_BODY},
		{
			name: "empty list",
			flow: `{
				"functions": [{"name": "f", "statements": {"statement": []}}],
				"main": {"statement": [{"call": {"function": "f"}}]}
			}`,
			err: graph.ERR_NO_BODY,
		},
		{name: "comments", flow: _Bodied(t, "# only"), err: graph.ERR_NO_BODY},
		{
			name: "both",
			flow: `{"main": {"statement": [{"call": {
				"function": "f", "args": [1], "operands": [{"name": "x"}]
			}}]}}`,
			err: graph.ERR_FORM,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := graph.Emit(_Flow(t, tc.flow))
			if !errors.Is(err, tc.err) {
				t.Fatalf("got %v, want %v", err, tc.err)
			}
		})
	}
}

// TestEmit_RoundTrips writes each fixture's flow and derives it back: what is
// read is what was written.
//
// Revisions:
//   - 2026-10-02 00:18: initial creation
func TestEmit_RoundTrips(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"dag", "every", "shapes"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			want := _Flow(t, _Read(t, "testdata/"+name+".json"))

			out, err := graph.Emit(want)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}

			got := _Derived(t, string(out))

			_Same(t, got, want)
		})
	}
}
