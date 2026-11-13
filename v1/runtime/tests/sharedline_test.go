package tests

import (
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// SHARED_UNUSED is a script whose first line holds an argument that nothing
	// reads and a constant, two statements on one line.
	//
	// Revisions:
	//   - 2026-11-12 22:07: initial creation (C130, B285, Q400)
	SHARED_UNUSED = `
x = arg("a"); y = 2

def main():
    return y
`

	// SHARED_USED is SHARED_UNUSED with a main that reads the argument.
	//
	// Revisions:
	//   - 2026-11-12 22:07: initial creation (C130, B285, Q400)
	SHARED_USED = `
x = arg("a"); y = 2

def main():
    return x
`

	// SHARED_MIDDLE has an unread argument between two statements that are read.
	//
	// Revisions:
	//   - 2026-11-12 22:07: initial creation (C130, B285, Q400)
	SHARED_MIDDLE = `
y = 1; x = arg("a"); z = 3

def main():
    return y + z
`

	// SHARED_SPACED has an unread argument over two lines, and a space before the
	// semicolon that follows it.
	//
	// Revisions:
	//   - 2026-11-12 22:14: initial creation (C130, B285, Q400)
	SHARED_SPACED = `
x = [
    1] ; y = 2

def main():
    return y
`

	// SHARED_INDEXED has an unread constant that ends with an index, and a
	// slice and a parenthesis before the next, which the parser ends at the
	// closing bracket.
	//
	// Revisions:
	//   - 2026-11-12 22:51: initial creation (C130, B288, Q400)
	SHARED_INDEXED = `
x = [1][0]; z = [1][0:1] ; w = (1, 2); y = 2

def main():
    return y
`

	// SHARED_CONTINUED has an unread constant, a line continuation, and the
	// semicolon on the next line.
	//
	// Revisions:
	//   - 2026-11-12 23:35: initial creation (C130, B291)
	SHARED_CONTINUED = `
x = 1 \
; y = 2

def main():
    return y
`

	// SHARED_TRAILING has an unread constant that ends its line with a semicolon,
	// and the statement that is read on the next line: the line is not shared.
	//
	// Revisions:
	//   - 2026-11-13 00:45: initial creation (C130, B285)
	SHARED_TRAILING = `
x = 1;
y = 2

def main():
    return y
`

	// SHARED_COMMENTED is SHARED_TRAILING with a comment after the semicolon.
	//
	// Revisions:
	//   - 2026-11-13 00:45: initial creation (C130, B285)
	SHARED_COMMENTED = `
x = 1;  # a comment
y = 2

def main():
    return y
`

	// SHARED_TABBED ends a line with a semicolon and a tab, then a comment.
	//
	// Revisions:
	//   - 2026-11-13 01:19: initial creation (C130, B285)
	SHARED_TABBED = `
x = 1;	# a comment
y = 2

def main():
    return y
`

	// SHARED_TAB_END ends a line with a semicolon and a tab, and nothing after.
	//
	// Revisions:
	//   - 2026-11-13 01:19: initial creation (C130, B285)
	SHARED_TAB_END = `
x = 1;	
y = 2

def main():
    return y
`

	// SHARED_ALONE has the semicolon of a statement alone on the line after a line
	// continuation, and a statement on the next line.
	//
	// Revisions:
	//   - 2026-11-13 01:19: initial creation (C130, B292)
	SHARED_ALONE = `
x = 1 \
;
y = 2

def main():
    return y
`

	// SHARED_ALONE_NOTED is SHARED_ALONE with a comment after the semicolon.
	//
	// Revisions:
	//   - 2026-11-13 01:19: initial creation (C130, B292)
	SHARED_ALONE_NOTED = `
x = 1 \
;  # a comment
y = 2

def main():
    return y
`

	// SHARED_ALONE_LAST ends the text with a continuation and a semicolon alone, no
	// line feed after it.
	//
	// Revisions:
	//   - 2026-11-13 01:19: initial creation (C130, B292)
	SHARED_ALONE_LAST = "y = 2\n\ndef main():\n    return y\nx = 1 \\\n;"

	// SHARED_LAST has an unread argument after a statement that is read.
	//
	// Revisions:
	//   - 2026-11-12 22:07: initial creation (C130, B285, Q400)
	SHARED_LAST = `
y = 1; x = arg("a")

def main():
    return y
`
)

// TestRun_ACutStatementOnASharedLineIsNotRun proves the compile shakes per
// statement (Q400): an argument nothing reads is not required of the run when
// another statement shares its line, and the flow lists no such argument, so a
// script and the flow derived from it run alike. An argument that is read stays
// required.
//
// The check: go test -race -count=1 -run TestRun_ACutStatementOnASharedLineIsNotRun
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-12 22:07: initial creation (C130, B285, Q400)
func TestRun_ACutStatementOnASharedLineIsNotRun(t *testing.T) {
	var tests = []struct {
		name    string
		text    string
		args    map[string]any
		wantErr bool
		wantArg bool
	}{
		{
			"an unread argument first on the line",
			SHARED_UNUSED,
			nil,
			false,
			false,
		},
		{
			"an unread argument in the middle of the line",
			SHARED_MIDDLE,
			nil,
			false,
			false,
		},
		{
			"an unread argument last on the line",
			SHARED_LAST,
			nil,
			false,
			false,
		},
		{
			"a read argument, not supplied",
			SHARED_USED,
			nil,
			true,
			true,
		},
		{
			"a read argument, supplied",
			SHARED_USED,
			map[string]any{"a": 7},
			false,
			true,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var source = &common.Source{
					Entry: "shared.star",
					Text: []byte(strings.TrimPrefix(
						tc.text,
						"\n",
					)),
				}

				_, err := runtime.Run(
					t.Context(),
					source,
					runtime.WithArgs(tc.args),
				)
				if (err != nil) != tc.wantErr {
					t.Fatalf(
						"run script: err = %v, want error %v",
						err,
						tc.wantErr,
					)
				}

				flow, err := runtime.Derive(source)
				if err != nil {
					t.Fatal(err)
				}

				if got := len(flow.GetArgs()) > 0; got != tc.wantArg {
					t.Fatalf(
						"flow lists an argument: got %v, want %v",
						got,
						tc.wantArg,
					)
				}

				text, err := runtime.Emit(flow)
				if err != nil {
					t.Fatal(err)
				}

				_, err = runtime.Run(
					t.Context(),
					&common.Source{
						Entry: "shared_flow.star",
						Text:  text,
					},
					runtime.WithArgs(tc.args),
				)
				if (err != nil) != tc.wantErr {
					t.Fatalf(
						"run flow: err = %v, want error %v",
						err,
						tc.wantErr,
					)
				}
			},
		)
	}
}

// TestMerge_CutsAStatementOnASharedLineAndKeepsTheColumns proves the text of a
// merge writes an empty tuple over a shaken statement that another follows, with
// the same lines and columns, and keeps the semicolon, so every statement that
// stays keeps its place, a failure names the place its author wrote (Q400), and
// the text is a script that parses and runs as the original does: spaces alone
// would leave the next statement indented (B291). A statement that is last on its
// line goes to the end of the line, and so does one that ends with a semicolon
// and no statement after it, as at 6414851 (B285). A line continuation before the
// semicolon stays, with the cut statement before it (B291).
//
// The check: go test -race -count=1 -run TestMerge_CutsAStatementOnASharedLineAndKeepsTheColumns
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-12 23:38: initial creation (C130, B285, B291, Q400)
func TestMerge_CutsAStatementOnASharedLineAndKeepsTheColumns(t *testing.T) {
	var tests = []struct {
		name string
		text string
		want string
		got  string
	}{
		{
			"first on the line",
			SHARED_UNUSED,
			"(          ); y = 2",
			"2",
		},
		{
			"in the middle of the line",
			SHARED_MIDDLE,
			"y = 1; (          ); z = 3",
			"4",
		},
		{
			"last on the line",
			SHARED_LAST,
			"y = 1; ",
			"1",
		},
		{
			"constants that end with an index, a slice and a parenthesis",
			SHARED_INDEXED,
			"(        ); (          ) ; (        ); y = 2",
			"2",
		},
		{
			"over two lines, a space before the semicolon",
			SHARED_SPACED,
			"(    \n     ) ; y = 2",
			"2",
		},
		{
			"a semicolon at the end of the line is no shared line",
			SHARED_TRAILING,
			"\ny = 2",
			"2",
		},
		{
			"a semicolon before a comment is no shared line",
			SHARED_COMMENTED,
			"\ny = 2",
			"2",
		},
		{
			"a semicolon and a tab, then a comment, is no shared line",
			SHARED_TABBED,
			"\ny = 2",
			"2",
		},
		{
			"a semicolon and a tab at the end of the line is no shared line",
			SHARED_TAB_END,
			"\ny = 2",
			"2",
		},
		{
			"a semicolon alone on the line after a continuation",
			SHARED_ALONE,
			"\n\ny = 2",
			"2",
		},
		{
			"a semicolon and a comment alone on the line after a continuation",
			SHARED_ALONE_NOTED,
			"\n\ny = 2",
			"2",
		},
		{
			"a line continuation before the semicolon",
			SHARED_CONTINUED,
			"(   ) \\\n; y = 2",
			"2",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				merged, err := runtime.Merge(&common.Source{
					Entry: "shared.star",
					Text: []byte(strings.TrimPrefix(
						tc.text,
						"\n",
					)),
				})
				if err != nil {
					t.Fatal(err)
				}

				first, _, _ := strings.Cut(
					string(merged.Text),
					"\n\ndef main",
				)
				if first != tc.want {
					t.Fatalf(
						"merged text: got %q, want %q",
						first,
						tc.want,
					)
				}

				got, err := runtime.Run(
					t.Context(),
					merged,
				)
				if err != nil {
					t.Fatalf(
						"run the merged text: %v",
						err,
					)
				}

				if got.String() != tc.got {
					t.Fatalf(
						"run the merged text: got %v, want %v",
						got,
						tc.got,
					)
				}
			},
		)
	}
}

// TestMerge_ASemicolonAloneAtTheEndOfTheTextParses proves the cut of a statement
// whose semicolon stands alone on the last line, with no line feed after it,
// leaves a text that parses, runs as the script does, and has the lines of the
// script (B292).
//
// The check: go test -race -count=1 -run TestMerge_ASemicolonAloneAtTheEndOfTheTextParses
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 01:19: initial creation (C130, B292)
func TestMerge_ASemicolonAloneAtTheEndOfTheTextParses(t *testing.T) {
	merged, err := runtime.Merge(&common.Source{
		Entry: "alone.star",
		Text:  []byte(SHARED_ALONE_LAST),
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := runtime.Run(
		t.Context(),
		merged,
	)
	if err != nil || got.String() != "2" {
		t.Fatalf(
			"run the merged text: %v, %v\n%s",
			got,
			err,
			merged.Text,
		)
	}

	want := strings.Count(
		SHARED_ALONE_LAST,
		"\n",
	) + 1 // the merge ends the text with a line feed
	if lines := strings.Count(
		string(merged.Text),
		"\n",
	); lines != want {
		t.Fatalf(
			"merged text has %d line feeds, the script %d:\n%q",
			lines,
			want,
			merged.Text,
		)
	}
}
