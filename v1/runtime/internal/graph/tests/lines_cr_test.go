package tests

import (
	"errors"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// TestLines_BreakWhereTheScannerBreaks proves the line table splits a text
// where the scanner of the parser does: at a line feed, at a carriage return
// followed by a line feed, and at a carriage return alone ("Newlines in Unix,
// DOS, or Mac format are treated as one rune", scan.go readRune in the pinned
// go.starlark.net). The line of a position the parser gives is then a line of
// the table (B260).
//
// A CRLF line keeps its carriage return, as the table always held it, so a
// slice of a DOS file is the text the author wrote.
//
// Revisions:
//   - 2026-11-10 11:57: initial creation (C118, B260)
func TestLines_BreakWhereTheScannerBreaks(t *testing.T) {
	var tests = []struct {
		name string
		src  string
		line int32
		col  int32
		at   int
		end  int
	}{
		{
			"line feed, second line",
			"ab\ncd",
			2,
			1,
			3,
			5,
		},
		{
			"line feed, first line end",
			"ab\ncd",
			1,
			1,
			0,
			2,
		},
		{
			"lone carriage return, second line",
			"ab\rcd",
			2,
			1,
			3,
			5,
		},
		{
			"lone carriage return, first line end",
			"ab\rcd",
			1,
			1,
			0,
			2,
		},
		{
			"carriage return and line feed, second line",
			"ab\r\ncd",
			2,
			1,
			4,
			6,
		},
		{
			"carriage return and line feed, keeps the return",
			"ab\r\ncd",
			1,
			1,
			0,
			3,
		},
		{
			"two lone returns, third line",
			"a\rb\rc",
			3,
			1,
			4,
			5,
		},
		{
			"lone return then line feed then text",
			"a\r\n\rb",
			3,
			1,
			4,
			5,
		},
		{
			"trailing lone return opens an empty line",
			"ab\r",
			2,
			1,
			3,
			3,
		},
		{
			"no such line",
			"ab\rcd",
			3,
			1,
			graph.MISSING,
			graph.MISSING,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					text = graph.NewLines(tc.src)
					got  = text.At(
						tc.line,
						tc.col,
					)
				)
				if got != tc.at {
					t.Fatalf(
						"At(%d, %d) got %d, want %d",
						tc.line,
						tc.col,
						got,
						tc.at,
					)
				}

				got = text.End(tc.line)
				if got != tc.end {
					t.Fatalf(
						"End(%d) got %d, want %d",
						tc.line,
						got,
						tc.end,
					)
				}
			},
		)
	}
}

// TestOf_ALoneReturnInAStatementIsNoPanic proves Of answers a top-level
// statement that spans a lone carriage return, as the scanner reads it, with
// a flow or an error and never a panic (B260, Q2, Q65). The statement is
// carried, and the script it writes derives.
//
// Revisions:
//   - 2026-11-10 11:57: initial creation (C118, B260)
//   - 2026-11-10 17:15: uses ErrMain, formerly ErrNoMain (C121, Q348)
func TestOf_ALoneReturnInAStatementIsNoPanic(t *testing.T) {
	var tests = []struct {
		name string
		text string
		fail error
	}{
		{
			"an empty tuple spanning a return, no main",
			"(\r)",
			spelling.ErrMain,
		},
		{
			"an empty tuple spanning a return, with main",
			"(\r)\ndef main():\n    pass\n",
			nil,
		},
		{
			"a list spanning a return",
			"x = [\r]\ndef main():\n    print(x)\n",
			nil,
		},
		{
			"a call spanning a return",
			"print(\r1)\ndef main():\n    pass\n",
			nil,
		},
		{
			"a call spanning a return and a line feed",
			"print(\r\n1,\r2)\ndef main():\n    pass\n",
			nil,
		},
		{
			"a statement after a line of returns",
			"\r\r\rprint(1)\ndef main():\n    pass\n",
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := graph.Of(&common.Source{
					Entry: "z.star",
					Text:  []byte(tc.text),
				})
				if !errors.Is(
					err,
					tc.fail,
				) || (tc.fail == nil && err != nil) {
					t.Fatalf(
						"derive: got %v, want %v",
						err,
						tc.fail,
					)
				}

				if err != nil {
					return
				}

				out, err := graph.Emit(flow)
				if err != nil {
					t.Fatalf(
						"emit: %v",
						err,
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

// TestReading_PlacedRefusesATextThatEndsEarly proves Placed answers a
// statement whose last line the text lacks with ErrCarry and not a
// slice-bounds panic, whatever made the text and the tree disagree (B260, Q65,
// Q206). A caller can hand Placed a text that is not the one the tree was
// parsed from, so the line table is not the only way to reach this.
//
// Revisions:
//   - 2026-11-10 11:58: initial creation (C118, B260)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func TestReading_PlacedRefusesATextThatEndsEarly(t *testing.T) {
	file, err := common.OPTIONS.Parse(
		"z.star",
		"x = [\n1]\n",
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	err = new(graph.Reading).Placed(
		file.Stmts[0],
		"",
		graph.NewLines("x = ["),
	)
	if !errors.Is(
		err,
		graph.ErrCarry,
	) {
		t.Fatalf(
			"got %v, want ErrCarry",
			err,
		)
	}
}
