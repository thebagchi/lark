package tests

import (
	"errors"
	"strings"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestOf_NeverTurnsBytesIntoAString proves a bytes literal is not read as the
// string with the same characters: go.starlark.net stores the value of a bytes
// literal as a Go string, so a read of its value alone cannot tell the two
// apart (B290). A flow value has a bytes kind (Q407), so a constant that holds
// bytes, alone or in a list or a dict however deep, an argument with a bytes
// default, and a bytes argument of a call are real values of the flow, listed
// where a string would be, and none is init text. Only a dict with a key that is
// no string, a bytes key among them, is still carried as the text of an init
// statement (Q406, Q423). A tuple is no value a flow holds, bytes or not, and is
// still refused. A match on bytes is no match.
//
// The check: go test -count=1 -run TestOf_NeverTurnsBytesIntoAString
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 00:44: initial creation (C130, B290, Q403, Q406)
//   - 2026-11-13 10:15: a bytes value is a value of the flow, not init text; a dict with a
//     key that is no string is the text that is left (C132, Q406, Q407, Q423)
func TestOf_NeverTurnsBytesIntoAString(t *testing.T) {
	var tests = []struct {
		name string
		text string
		err  error
		want string
		held string
	}{
		{
			"bytes in a list constant",
			"X = [b\"\"\"0\"\"\"]\ndef main():\n    (X)\n",
			nil,
			"X = [b\"0\"]",
			"",
		},
		{
			"top-level bytes constant",
			"X = b\"0\"\ndef main():\n    (X)\n",
			nil,
			"X = b\"0\"",
			"",
		},
		{
			"raw bytes constant",
			"X = rb\"0\"\ndef main():\n    (X)\n",
			nil,
			"X = b\"0\"",
			"",
		},
		{
			"bytes in a nested list constant",
			"X = [[1, [b\"a\"]]]\ndef main():\n    (X)\n",
			nil,
			"X = [[1, [b\"a\"]]]",
			"",
		},
		{
			"bytes as a dict value in a constant",
			"X = {\"k\": b\"v\"}\ndef main():\n    (X)\n",
			nil,
			"X = {\"k\": b\"v\"}",
			"",
		},
		{
			"bytes as a dict key in a constant",
			"X = {b\"k\": 1}\ndef main():\n    (X)\n",
			nil,
			"X = {b\"k\": 1}",
			"X = {b\"k\": 1}",
		},
		{
			"a dict with a number as its key is text, as one with a bytes key is",
			"X = {1: \"x\"}\ndef main():\n    (X)\n",
			nil,
			"X = {1: \"x\"}",
			"X = {1: \"x\"}",
		},
		{
			"a dict with a key that is a name is no value",
			"k = 1\nX = {k: \"x\"}\ndef main():\n    (X)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"an argument whose default is a dict with a key that is no string",
			"a = arg(\"a\", {1: 2})\ndef main():\n    (a)\n",
			nil,
			"a = arg(\"a\", {1: 2})",
			"a = arg(\"a\", {1: 2})",
		},
		{
			"a constant that is a call with a bytes argument",
			"def f(a):\n    return a\nX = f(b\"x\")\ndef main():\n    (X)\n",
			nil,
			"X = f(b\"x\")",
			"",
		},
		{
			"a call constant with a sum of bytes is no value",
			"def f(a):\n    return a\nX = f(b\"x\" + b\"y\")\ndef main():\n    (X)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"a constant that is a call with a list of bytes and a string",
			"def f(a, b):\n    return a\nX = f([b\"x\"], \"s\")\n" +
				"def main():\n    (X)\n",
			nil,
			"X = f([b\"x\"], \"s\")",
			"",
		},
		{
			"a call constant with a bytes keyword value is refused like a string",
			"def f(a):\n    return a\nX = f(a=b\"x\")\ndef main():\n    (X)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"a constant that is a call with a keyword string value",
			"def f(a):\n    return a\nX = f(a=\"x\")\ndef main():\n    (X)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"a call of a function the flow lacks, with bytes, is refused",
			"X = g(b\"x\")\ndef main():\n    (X)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"a string call constant is still a constant",
			"def f(a):\n    return a\nX = f(\"x\")\ndef main():\n    (X)\n",
			nil,
			"X = f(\"x\")",
			"",
		},
		{
			"bytes in a tuple constant",
			"X = (b\"a\", 1)\ndef main():\n    (X)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"an argument with a bytes default",
			"a = arg(\"a\", b\"d\")\ndef main():\n    (a)\n",
			nil,
			"a = arg(\"a\", b\"d\")",
			"",
		},
		{
			"an argument with a bytes default given as a keyword",
			"a = arg(\"a\", default=b\"d\")\ndef main():\n    (a)\n",
			nil,
			"a = arg(\"a\", b\"d\")",
			"",
		},
		{
			"an argument with three arguments, one a bytes default",
			"a = arg(\"a\", b\"d\", 3)\ndef main():\n    (a)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"an argument named as a keyword, with a bytes default",
			"a = arg(name=\"a\", default=b\"d\")\ndef main():\n    (a)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"an argument named as a keyword, with a string default",
			"a = arg(name=\"a\", default=\"d\")\ndef main():\n    (a)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"an argument with an empty name and a bytes default",
			"a = arg(\"\", b\"d\")\ndef main():\n    (a)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"an argument with a name that is bytes and a bytes default",
			"a = arg(b\"a\", b\"d\")\ndef main():\n    (a)\n",
			graph.ErrCarry,
			"",
			"",
		},
		{
			"a call of an arg the script defines is no declaration",
			"def arg(n, d):\n    return d\nX = arg(\"a\", b\"d\")\n" +
				"def main():\n    (X)\n",
			nil,
			"X = arg(\"a\", b\"d\")",
			"",
		},
		{
			"an argument with a default that holds bytes in a list",
			"a = arg(\"a\", [b\"d\"])\ndef main():\n    (a)\n",
			nil,
			"a = arg(\"a\", [b\"d\"])",
			"",
		},
		{
			"a string list constant is still a constant",
			"X = [\"0\"]\ndef main():\n    (X)\n",
			nil,
			"X = [\"0\"]",
			"",
		},
		{
			"a string default is still an argument",
			"a = arg(\"a\", \"d\")\ndef main():\n    (a)\n",
			nil,
			"a = arg(\"a\", \"d\")",
			"",
		},
		{
			"bytes as an argument of a call in main",
			"def A(x):\n    return x\ndef main():\n    A(b\"\\\\x00\")\n",
			nil,
			"A(b\"\\x5cx00\")",
			"",
		},
		{
			"bytes in a list argument of a call in main",
			"def A(x):\n    return x\ndef main():\n    A([b\"a\", \"b\"])\n",
			nil,
			"A([b\"a\", \"b\"])",
			"",
		},
		{
			"bytes as the subject of a match",
			"def f():\n    return 1\ndef main():\n    _match = b\"a\"\n" +
				"    if _match == \"a\":\n        f()\n" +
				"    elif _match == \"b\":\n        f()\n    else:\n        f()\n",
			nil,
			"_match = b\"a\"",
			"",
		},
		{
			"bytes as an arm of a match",
			"def f():\n    return 1\ndef main():\n    _match = \"a\"\n" +
				"    if _match == b\"a\":\n        f()\n    else:\n        f()\n",
			nil,
			"if _match == b\"a\":",
			"",
		},
		{
			"bytes as an argument of an init call",
			"def A(x):\n    return x\nA(b\"\")\ndef main():\n    pass\n",
			nil,
			"A(b\"\")",
			"",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := graph.Of(&common.Source{
					Entry: "script.star",
					Text:  []byte(tc.text),
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

				out, err := graph.Emit(flow)
				if err != nil {
					t.Fatalf(
						"emit: %v",
						err,
					)
				}

				if !strings.Contains(
					string(out),
					tc.want,
				) {
					t.Fatalf(
						"script holds %q: got %q",
						tc.want,
						out,
					)
				}

				if got := _Initial(flow); got != tc.held {
					t.Fatalf(
						"init text: got %q, want %q",
						got,
						tc.held,
					)
				}

				listed := len(flow.GetArgs()) + len(flow.GetConstants())
				if tc.held != "" && listed > 0 {
					t.Fatalf(
						"the flow lists %v and %v besides the init text",
						flow.GetArgs(),
						flow.GetConstants(),
					)
				}

				if err := graph.Check(flow); err != nil {
					t.Fatalf(
						"check: %v",
						err,
					)
				}
			},
		)
	}
}

// _Initial is the text of the first init statement of a flow that is text, or
// empty.
//
// Revisions:
//   - 2026-11-13 00:44: initial creation (C130, B290, Q406)
func _Initial(flow *workflowpb.Flow) string {
	for _, held := range flow.GetInit() {
		if held.GetText() != "" {
			return held.GetText()
		}
	}

	return ""
}
