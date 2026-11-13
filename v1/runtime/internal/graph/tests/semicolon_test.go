package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestSemicolon_SkipsOnlyWhatCanStandBetween proves Semicolon finds the
// semicolon after a statement from the offset the parser gives as its end, in a
// string and in bytes. That offset may stand on the closing bracket of an index,
// a slice or a parenthesis (B288), so closing brackets, spaces and tabs are
// skipped, and so is a backslash before a line break, a line continuation
// (B291); any other byte ends the search, so a semicolon in a comment or in a
// later statement is not taken. An offset below zero finds none (Q65).
//
// The check: go test -count=1 -run TestSemicolon_SkipsOnlyWhatCanStandBetween
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 22:50: initial creation (C130, B288)
func TestSemicolon_SkipsOnlyWhatCanStandBetween(t *testing.T) {
	var tests = []struct {
		name  string
		text  string
		at    int
		after int
		found bool
	}{
		{
			"at the semicolon",
			"0;1",
			1,
			2,
			true,
		},
		{
			"blanks before it",
			"0 \t;1",
			1,
			4,
			true,
		},
		{
			"on a closing bracket of an index",
			"a[0];1",
			3,
			5,
			true,
		},
		{
			"on a closing parenthesis, blanks after it",
			"();1",
			1,
			3,
			true,
		},
		{
			"two closing brackets",
			"a[b[0]] ;1",
			6,
			9,
			true,
		},
		{
			"a comment first",
			"0 # a;b",
			1,
			1,
			false,
		},
		{
			"a line break first",
			"0\n;1",
			1,
			1,
			false,
		},
		{
			"the end of the text",
			"0 ",
			1,
			1,
			false,
		},
		{
			"an offset past the end of the text",
			"0",
			5,
			5,
			false,
		},
		{
			"a line continuation first",
			"0 \\\n;1",
			1,
			5,
			true,
		},
		{
			"a continuation with a carriage return and a line feed",
			"0 \\\r\n;1",
			1,
			6,
			true,
		},
		{
			"a continuation with a lone carriage return",
			"0\\\r;1",
			1,
			4,
			true,
		},
		{
			"a backslash before another byte",
			"0 \\x;1",
			1,
			1,
			false,
		},
		{
			"a backslash at the end of the text",
			"0\\",
			1,
			1,
			false,
		},
		{
			"a negative offset",
			"0 ;x",
			-1,
			-1,
			false,
		},
		{
			"another statement first",
			"0 1;2",
			1,
			1,
			false,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				after, found := graph.Semicolon(
					tc.text,
					tc.at,
				)
				if after != tc.after || found != tc.found {
					t.Fatalf(
						"string: got (%d, %v), want (%d, %v)",
						after,
						found,
						tc.after,
						tc.found,
					)
				}

				after, found = graph.Semicolon(
					[]byte(tc.text),
					tc.at,
				)
				if after != tc.after || found != tc.found {
					t.Fatalf(
						"bytes: got (%d, %v), want (%d, %v)",
						after,
						found,
						tc.after,
						tc.found,
					)
				}
			},
		)
	}
}

// TestFollowing_IsAStatementOnTheSameLogicalLine proves Following answers true
// only when a statement starts after the offset, past spaces, tabs and line
// continuations, in a string and in bytes: a comment, a line break, a backslash
// that is no continuation's start or the end of the text mean the semicolon
// before the offset joined nothing, and a line that ends with one is not a
// shared line (Q400). An offset below zero finds none (Q65).
//
// The check: go test -count=1 -run TestFollowing_IsAStatementOnTheSameLogicalLine
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 00:45: initial creation (C130, B285)
func TestFollowing_IsAStatementOnTheSameLogicalLine(t *testing.T) {
	var tests = []struct {
		name string
		text string
		at   int
		want bool
	}{
		{
			"a statement",
			";y = 2",
			1,
			true,
		},
		{
			"after blanks",
			";  \ty = 2",
			1,
			true,
		},
		{
			"over a continuation",
			"; \\\ny = 2",
			1,
			true,
		},
		{
			"over a continuation with a carriage return",
			";\\\r\ny",
			1,
			true,
		},
		{
			"over two continuations",
			";\\\n\\\ny",
			1,
			true,
		},
		{
			"a backslash that continues nothing",
			";\\x",
			1,
			true,
		},
		{
			"the end of the text",
			";",
			1,
			false,
		},
		{
			"blanks to the end of the text",
			";  ",
			1,
			false,
		},
		{
			"a line feed",
			";\ny = 2",
			1,
			false,
		},
		{
			"a carriage return",
			";\ry = 2",
			1,
			false,
		},
		{
			"a comment",
			"; # c\ny = 2",
			1,
			false,
		},
		{
			"a comment right after",
			";#c",
			1,
			false,
		},
		{
			"a continuation, then a line end",
			"; \\\n\ny",
			1,
			false,
		},
		{
			"a continuation, then a comment",
			"; \\\n# c",
			1,
			false,
		},
		{
			"an offset past the end",
			";",
			5,
			false,
		},
		{
			"a negative offset",
			"x",
			-1,
			false,
		},
		{
			"a tab, then a comment",
			";\t# c",
			1,
			false,
		},
		{
			"a tab, then a line end",
			";\t\ny",
			1,
			false,
		},
		{
			"a tab, then a statement",
			";\ty",
			1,
			true,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				if got := graph.Following(
					tc.text,
					tc.at,
				); got != tc.want {
					t.Fatalf(
						"string: got %v, want %v",
						got,
						tc.want,
					)
				}

				if got := graph.Following(
					[]byte(tc.text),
					tc.at,
				); got != tc.want {
					t.Fatalf(
						"bytes: got %v, want %v",
						got,
						tc.want,
					)
				}
			},
		)
	}
}
