package tests

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

const (
	// FLOW_HEAD opens a flow whose function f has a body. The body goes after
	// it, as a JSON string, and FLOW_TAIL ends the flow.
	//
	// Revisions:
	//   - 2026-11-11 00:31: initial creation (C122, B264)
	FLOW_HEAD = `{"functions": [{"name": "f", "body": "`

	// FLOW_TAIL ends a flow that FLOW_HEAD opened, with a main that calls f.
	//
	// Revisions:
	//   - 2026-11-11 22:33: initial creation (C122, B264)
	FLOW_TAIL = `"}], "main": {"statement": [{"call": {"function": "f"}}]}}`
)

// TestOf_RefusesWhatEmitCannotWrite proves Of answers an arg() with an empty
// name with ErrCarry, so a flow Of derives is one Emit writes: "So a flow is
// the whole script" (the doc of Of). A name is how a run is handed a value, and
// a flow keeps an unset name apart from an empty one by refusing both (B262).
//
// Revisions:
//   - 2026-11-11 00:31: initial creation (C122, B262)
func TestOf_RefusesWhatEmitCannotWrite(t *testing.T) {
	var tests = []struct {
		name string
		text string
	}{
		{
			"an empty name",
			`
r = arg("")

def main():
    print(r)
`,
		},
		{
			"an empty name with a default",
			`
r = arg("", 1)

def main():
    print(r)
`,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := graph.Of(&common.Source{
					Entry: "z.star",
					Text: []byte(strings.TrimPrefix(
						tc.text,
						"\n",
					)),
				})
				if !errors.Is(
					err,
					graph.ErrCarry,
				) {
					_, emitted := graph.Emit(flow)

					t.Fatalf(
						"derive: got %v, want ErrCarry (emit: %v)",
						err,
						emitted,
					)
				}
			},
		)
	}
}

// TestOf_IsAFixedPointOfEmit proves a text is shaken the first time it is
// read: the script Emit writes from the flow Of derives derives the same flow
// and writes the same script again, whatever the scanner of the parser makes of
// the text. A NUL ends the file for the scanner, and a lone carriage return is
// a line break to it, so such a text was read whole, with a def nothing calls
// kept, and the second reading left it out (B263). A lone return inside a bracket
// of a body on the header's line is a line feed after the first reading, as it is
// in a body under the header (B264). A comment on a line of its own inside a
// bracket is part of the statement, and a trailing comment after it does not
// write it a second time (B276). Two statements on one line are shaken apart,
// as the script Emit writes puts each on a line of its own (B285), and a comment
// right after an opening bracket is trimmed the first time, as the end-of-line
// comments are (B286, Q393). A statement that another follows on its line is
// written once, with the text of the statement alone (B288).
//
// Revisions:
//   - 2026-11-11 00:31: initial creation (C122, B263, B264)
//   - 2026-11-12 18:00: cases for a comment inside a bracket, an init statement over
//     several lines, and a string over several lines (C128, B276, B279, B282)
//   - 2026-11-12 22:11: cases for two statements on a line, a comment after an opening
//     bracket, and an init statement before a call on its line (C130, B285, B286, B288)
func TestOf_IsAFixedPointOfEmit(t *testing.T) {
	var tests = []struct {
		name string
		text string
	}{
		{
			"a NUL after an uncalled def",
			"def main():0\ndef A():0\x00",
		},
		{
			"a NUL and a line feed after an uncalled def",
			"def main():0\ndef A():0\x00\n",
		},
		{
			"a NUL after main",
			"def main():0\x00\ndef A():0\n",
		},
		{
			"an uncalled def, no NUL",
			"def main():0\ndef A():0\n",
		},
		{
			"a lone return, an uncalled def",
			"def main():\r    pass\rdef A():\r    pass\r",
		},
		{
			"a lone return inside a bracket of a one-line body",
			"def main():    f(1\r)\n",
		},
		{
			"a lone return inside a bracket of a one-line body, no helper",
			"def main():    print(1\r)\n",
		},
		{
			"a comment inside a bracket, a trailing comment",
			"def main():\n    f(\n#c\n)#d\n",
		},
		{
			"a comment inside a bracket after a value, a trailing comment",
			"def main():\n    f(1, # inside\n      2)  # trailing\n",
		},
		{
			"a comment on a line of its own inside a bracket, a statement after",
			"def main():\n    f(1,\n    # inside\n      2)\n    g()\n",
		},
		{
			"a comment on a line of its own inside a bracket of a called def",
			"def A():\n    f(\n    # inside\n    )  # trailing\n" +
				"def main():\n    A()\n",
		},
		{
			"an init statement over two lines",
			"[\n]\ndef main():()",
		},
		{
			"an init statement with a string over two lines",
			"[ '''a\nb''' ]\ndef main():\n    pass\n",
		},
		{
			"an init statement with comments in a bracket and after it",
			"[1, # first\n  2]  # last\ndef main():\n    pass\n",
		},
		{
			"a string over three lines in main",
			"def main():\n    y = '''a\nb\n    c'''\n    return y\n",
		},
		{
			"a lone return, a called def",
			"def A():\r    pass\rdef main():\r    A()\r",
		},
		{
			"two unused init statements on one line",
			"x = 1; y = 2\ndef main():\n    pass\n",
		},
		{
			"two unused init statements on one line, a third on the next",
			"x = 1; y = 2\nz = 3\ndef main():\n    pass\n",
		},
		{
			"an unused init statement beside a call on one line",
			"x = 1; print(2)\ndef main():\n    pass\n",
		},
		{
			"an init expression, then a call on its line",
			"0;print(\"x\")\ndef main():\n    pass\n",
		},
		{
			"an init call, then another on its line",
			"print(1);print(2)\ndef main():\n    pass\n",
		},
		{
			"a line continuation before the semicolon",
			"x = [1] \\\n; print(x[0])\ndef main():\n    pass\n",
		},
		{
			"a bytes constant in a list",
			"X = [b\"0\"]\ndef main():\n    (X)\n",
		},
		{
			"a bytes constant, a dict of bytes, and an argument with a bytes default",
			"X = b\"0\"\nY = {\"k\": b\"v\"}\na = arg(\"a\", b\"d\")\n" +
				"def main():\n    (X, Y, a)\n",
		},
		{
			"a bytes constant before a call on its line",
			"X = [b\"0\"]; print(X)\ndef main():\n    pass\n",
		},
		{
			"an index, then a call",
			"xs = [1]\nxs[0];print(\"x\")\ndef main():\n    pass\n",
		},
		{
			"a slice, then a call",
			"xs = [1]\nxs[0:1];print(\"x\")\ndef main():\n    pass\n",
		},
		{
			"an empty tuple, then a call",
			"();print(\"x\")\ndef main():\n    pass\n",
		},
		{
			"three statements on a line",
			"print(1); print((2)); print([3][0]); 0[0]\ndef main():\n    pass\n",
		},
		{
			"an init statement with a comment, then a call on its line",
			"0;print(\"x\") # c\ndef main():\n    pass\n",
		},
		{
			"a comment right after an opening bracket",
			"def main():(#\n0#\n)",
		},
		{
			"a comment right after an opening bracket, a space before the next",
			"def main():\n    f(#a\n      1 #b\n    )\n",
		},
		{
			"a comment after a bracket, then one on a line of its own",
			"def main():\n    f(#a\n    # own\n      1 #b\n    )\n",
		},
		{
			"a comment after a bracket, then a string with a hash",
			"def main():\n    f(#a\n      '#x'#b\n    )\n",
		},
		{
			"a comment after each of two opening brackets",
			"def main():\n    f(#a\n      [#b\n        1#c\n      ]#d\n    )\n",
		},
		{
			"a comment right after an opening bracket in an init statement",
			"print(#a\n  1#b\n)\ndef main():\n    pass\n",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					first = _Written(
						t,
						tc.text,
					)
					second = _Written(
						t,
						first,
					)
				)

				if first != second {
					t.Fatalf(
						"emit, derive, emit: got %q, then %q",
						first,
						second,
					)
				}
			},
		)
	}
}

// _Written is the script Emit writes from the flow Of derives from text.
//
// Revisions:
//   - 2026-11-11 00:31: initial creation (C122, B263)
func _Written(
	t *testing.T,
	text string,
) string {
	t.Helper()

	var (
		flow = _Derived(
			t,
			text,
		)
		out, err = graph.Emit(flow)
	)
	if err != nil {
		t.Fatalf(
			"emit: %v",
			err,
		)
	}

	return string(out)
}

// TestEmit_WritesAnInitStatementOnceOnASharedLine proves a statement that
// shares its line with another is written once: the text the flow keeps for an
// init statement ends where the statement ends, not at the end of its line, so
// the call after a semicolon is not in it as well (B288).
//
// The check: go test -count=1 -run TestEmit_WritesAnInitStatementOnceOnASharedLine
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 22:11: initial creation (C130, B288, Q401)
func TestEmit_WritesAnInitStatementOnceOnASharedLine(t *testing.T) {
	var tests = []struct {
		name string
		text string
		want string
	}{
		{
			"an expression, then a call",
			"0;print(\"x\")\ndef main():\n    pass\n",
			"0\nprint(\"x\")\n\ndef main():\n    pass\n",
		},
		{
			"two calls",
			"print(1);print(2)\ndef main():\n    pass\n",
			"print(1)\nprint(2)\n\ndef main():\n    pass\n",
		},
		{
			"two calls, a comment at the end of the line",
			"print(1); print(2)  # c\ndef main():\n    pass\n",
			"print(1)\nprint(2)\n\ndef main():\n    pass\n",
		},
		{
			"a trailing semicolon, a comment",
			"0;  # c\ndef main():\n    pass\n",
			"0\n\ndef main():\n    pass\n",
		},
		{
			"a space before the semicolon",
			"0 ;print(\"x\")\ndef main():\n    pass\n",
			"0\nprint(\"x\")\n\ndef main():\n    pass\n",
		},
		{
			"an index, then a call",
			"xs = [1]\nxs[0];print(\"x\")\ndef main():\n    pass\n",
			"xs = [1]\nxs[0]\nprint(\"x\")\n\ndef main():\n    pass\n",
		},
		{
			"a slice, a space before the semicolon, then a call",
			"xs = [1]\nxs[0:1] ; print(\"x\")\ndef main():\n    pass\n",
			"xs = [1]\nxs[0:1]\nprint(\"x\")\n\ndef main():\n    pass\n",
		},
		{
			"an empty tuple, then a call",
			"();print(\"x\")\ndef main():\n    pass\n",
			"()\nprint(\"x\")\n\ndef main():\n    pass\n",
		},
		{
			"an index of a literal, then a string",
			"0[0];''\ndef main():\n    pass\n",
			"0[0]\n''\n\ndef main():\n    pass\n",
		},
		{
			"a call of a method, then two calls",
			"a = {}\na.get(1);print(2);print(3)\ndef main():\n    pass\n",
			"a = {}\na.get(1)\nprint(2)\nprint(3)\n\ndef main():\n    pass\n",
		},
		{
			"a line continuation before the semicolon",
			"0 \\\n; print(2)\ndef main():\n    pass\n",
			"0\nprint(2)\n\ndef main():\n    pass\n",
		},
		{
			"three expressions",
			"1;2;3\ndef main():\n    pass\n",
			"1\n2\n3\n\ndef main():\n    pass\n",
		},
		{
			"a statement over two lines, then a call",
			"x = [\n  1]; print(2)\ndef main():\n    pass\n",
			"print(2)\n\ndef main():\n    pass\n",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := _Written(
					t,
					tc.text,
				)
				if got != tc.want {
					t.Fatalf(
						"emit: got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestOf_KeepsTheTextOfAnInitStatementToItsSemicolon proves the text a flow
// keeps for an init statement that is no call. A statement last on its line
// keeps what follows it on the line: its own semicolon, and the comment, which
// Emit trims. A statement that another follows on the line ends before the
// semicolon, whatever node it ends with, an index or a parenthesis among them,
// as the parser ends those at the closing bracket (B288).
//
// The check: go test -count=1 -run TestOf_KeepsTheTextOfAnInitStatementToItsSemicolon
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 22:50: initial creation (C130, B288)
func TestOf_KeepsTheTextOfAnInitStatementToItsSemicolon(t *testing.T) {
	var tests = []struct {
		name string
		text string
		want []string
	}{
		{
			"a trailing semicolon",
			"0;\ndef main():\n    pass\n",
			[]string{"0;"},
		},
		{
			"a trailing semicolon and a comment",
			"0; # c\ndef main():\n    pass\n",
			[]string{"0; # c"},
		},
		{
			"a comment after blanks and a semicolon",
			"0 ; # c\ndef main():\n    pass\n",
			[]string{"0 ; # c"},
		},
		{
			"a comment, no semicolon",
			"0 # c\ndef main():\n    pass\n",
			[]string{"0 # c"},
		},
		{
			"an expression, then a call",
			"0;print(1)\ndef main():\n    pass\n",
			[]string{"0", ""},
		},
		{
			"an index, blanks, then a call",
			"xs = [1]\nxs[0] ;  print(1)\ndef main():\n    pass\n",
			[]string{"xs[0]", ""},
		},
		{
			"a slice, then a string",
			"xs = [1]\nxs[0:1];''\ndef main():\n    pass\n",
			[]string{"xs[0:1]", "''"},
		},
		{
			"an empty tuple, then a call",
			"();print(1)\ndef main():\n    pass\n",
			[]string{"()", ""},
		},
		{
			"a nested index, then a string",
			"xs = [[1]]\nxs[0][0];''\ndef main():\n    pass\n",
			[]string{"xs[0][0]", "''"},
		},
		{
			"a parenthesised index, then a string",
			"xs = [1]\n(xs[0]);''\ndef main():\n    pass\n",
			[]string{"(xs[0])", "''"},
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					flow = _Derived(
						t,
						tc.text,
					)
					got []string
				)

				for _, held := range flow.GetInit() {
					got = append(
						got,
						held.GetText(),
					)
				}

				if !slices.Equal(
					got,
					tc.want,
				) {
					t.Fatalf(
						"init texts: got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestEmit_IndentsAfterALoneReturn proves a body that ends a line with a lone
// carriage return is indented line by line, as the scanner of the parser
// reads it, so the script Emit writes parses and keeps the body's blocks
// (B264). The line table is the one owner of where a line ends. A break between
// two statements is written as a line feed, and one inside a call stays.
//
// Revisions:
//   - 2026-11-11 00:31: initial creation (C122, B264)
func TestEmit_IndentsAfterALoneReturn(t *testing.T) {
	var tests = []struct {
		name string
		body string
		want string
	}{
		{
			"a block, then its line",
			"if x:\\r    y()",
			"def f():\n    if x:\n        y()\n        pass\n    pass\n",
		},
		{
			"two statements",
			"a()\\rb()",
			"def f():\n    a()\n    b()\n    pass\n",
		},
		{
			"a call over two lines",
			"x = (1,\\r2)\\nreturn x",
			"def f():\n    x = (1,\r    2)\n    return x\n",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					flow = _Flow(
						t,
						FLOW_HEAD+tc.body+FLOW_TAIL,
					)
					out, err = graph.Emit(flow)
				)
				if err != nil {
					t.Fatalf(
						"emit: %v",
						err,
					)
				}

				if !strings.HasPrefix(
					string(out),
					tc.want,
				) {
					t.Fatalf(
						"emit: got %q, want it to open with %q",
						out,
						tc.want,
					)
				}

				_Derived(
					t,
					string(out),
				)
			},
		)
	}
}

// TestEmit_ReadsLinesOfNotesAsTheScannerDoes proves a body is "only comments"
// when every line is blank or a comment, with a lone carriage return a line
// end as the scanner reads it: a body that opens with a comment and has a
// statement after a lone return is a body, and one of comments only is
// refused (B271, B264).
//
// Revisions:
//   - 2026-11-11 06:40: initial creation (C122, B271)
func TestEmit_ReadsLinesOfNotesAsTheScannerDoes(t *testing.T) {
	var tests = []struct {
		name string
		body string
		fail error
	}{
		{
			"a comment, a lone return, a statement",
			"# note\\rx = 1",
			nil,
		},
		{
			"a statement, a lone return, a comment",
			"x = 1\\r# note",
			nil,
		},
		{
			"comments only, divided by lone returns",
			"# a\\r# b\\r",
			graph.ErrForm,
		},
		{
			"comments only, divided by line feeds",
			"# a\\n# b\\n",
			graph.ErrForm,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					flow = _Flow(
						t,
						FLOW_HEAD+tc.body+FLOW_TAIL,
					)
					_, err = graph.Emit(flow)
				)
				if !errors.Is(
					err,
					tc.fail,
				) || (tc.fail == nil && err != nil) {
					t.Fatalf(
						"emit: got %v, want %v",
						err,
						tc.fail,
					)
				}
			},
		)
	}
}
