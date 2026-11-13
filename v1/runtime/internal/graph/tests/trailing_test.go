package tests

import (
	"errors"
	"slices"
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// _Retained parses text with its comments retained.
//
// Revisions:
//   - 2026-11-12 22:13: initial creation (C130, B286)
func _Retained(
	t *testing.T,
	text string,
) *syntax.File {
	t.Helper()

	file, err := common.OPTIONS.Parse(
		"trailing.star",
		text,
		syntax.RetainComments,
	)
	if err != nil {
		t.Fatalf(
			"parse: %v",
			err,
		)
	}

	return file
}

// TestTrailing_FindsEveryEndOfLineComment proves Trailing finds each comment
// that follows code on its line, in the order they appear, including the ones
// the parser hands to no node: a comment right after an opening bracket, with a
// comment that touches the token before it on the next line (B286). A comment
// on a line of its own is not one, and a hash in a string is no comment.
//
// The check: go test -count=1 -run TestTrailing_FindsEveryEndOfLineComment
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 22:13: initial creation (C130, B286)
func TestTrailing_FindsEveryEndOfLineComment(t *testing.T) {
	var tests = []struct {
		name string
		text string
		want []string
	}{
		{
			"none",
			"x = 1\n",
			nil,
		},
		{
			"one after a statement",
			"x = 1  # a\n",
			[]string{"# a"},
		},
		{
			"after an opening bracket, and touching the next token",
			"def _():\n    (#a\n0#b\n)\n",
			[]string{"#a", "#b"},
		},
		{
			"after each of two opening brackets",
			"f(#a\n  [#b\n    1#c\n  ]#d\n)\n",
			[]string{"#a", "#b", "#c", "#d"},
		},
		{
			"a comment on a line of its own is not one",
			"f(#a\n  # own\n  1 #b\n)\n",
			[]string{"#a", "#b"},
		},
		{
			"a hash in a string is no comment",
			"f(#a\n  '#x'#b\n)\n",
			[]string{"#a", "#b"},
		},
		{
			"a rune that is wider than a byte before the comment",
			"x = \"é\" # a\nf(#b\n\"é\"#c\n)\n",
			[]string{"# a", "#b", "#c"},
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				notes, err := graph.Trailing(
					"trailing.star",
					tc.text,
					_Retained(
						t,
						tc.text,
					),
				)
				if err != nil {
					t.Fatal(err)
				}

				var got []string

				for _, note := range notes {
					got = append(
						got,
						note.Text,
					)
				}

				if !slices.Equal(
					got,
					tc.want,
				) {
					t.Fatalf(
						"comments: got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestTrailing_RefusesATextThatIsNotTheFile proves Trailing answers a comment
// that is no part of the text with ErrForm, and a text that does not parse once
// its comments are blanked with an error, rather than cutting at a wrong place.
// A file that holds comments when the text holds no hash is ErrForm too: the
// loop is bound by the hashes of the text, so a blank that left one could not
// make it read for ever (Q65, Q206). No script reaches this: the file is the text parsed.
//
// The check: go test -count=1 -run TestTrailing_RefusesATextThatIsNotTheFile
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 22:13: initial creation (C130, B286, Q65)
func TestTrailing_RefusesATextThatIsNotTheFile(t *testing.T) {
	var tests = []struct {
		name string
		file string
		text string
		want error
	}{
		{
			"a comment past the end of the text",
			"x = 1\n\n\n  # a\ny = 2  # b\n",
			"x = 1  # a\n",
			graph.ErrForm,
		},
		{
			"comments in the file and no hash left in the text",
			"x = 1  # a\n",
			"x = 1     \n",
			graph.ErrForm,
		},
		{
			"a text that does not parse without its comment",
			"x = 1  # a\n",
			"x = (  # a\n",
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := graph.Trailing(
					"trailing.star",
					tc.text,
					_Retained(
						t,
						tc.file,
					),
				)
				if err == nil {
					t.Fatal("want an error")
				}

				if tc.want != nil && !errors.Is(
					err,
					tc.want,
				) {
					t.Fatalf(
						"err = %v, want %v",
						err,
						tc.want,
					)
				}
			},
		)
	}
}

// TestBlanked_KeepsEveryLineAndColumn proves Blanked writes spaces over the
// comments it is given and over nothing else, so the text keeps its length and
// each statement its place, and answers a comment that is no part of the text
// with ErrForm.
//
// The check: go test -count=1 -run TestBlanked_KeepsEveryLineAndColumn
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 22:13: initial creation (C130, B286, Q65)
func TestBlanked_KeepsEveryLineAndColumn(t *testing.T) {
	var (
		text  = "x = 1  # a\ny = \"é\"  # b\n"
		notes = []syntax.Comment{
			{
				Start: syntax.MakePosition(
					nil,
					1,
					8,
				),
				Text: "# a",
			},
			{
				Start: syntax.MakePosition(
					nil,
					2,
					10,
				),
				Text: "# b",
			},
		}
	)

	got, err := graph.Blanked(
		text,
		notes,
	)
	if err != nil {
		t.Fatal(err)
	}

	if want := "x = 1     \ny = \"é\"     \n"; got != want {
		t.Fatalf(
			"blanked: got %q, want %q",
			got,
			want,
		)
	}

	_, err = graph.Blanked(
		text,
		[]syntax.Comment{{
			Start: syntax.MakePosition(
				nil,
				9,
				1,
			),
			Text: "# c",
		}},
	)
	if !errors.Is(
		err,
		graph.ErrForm,
	) {
		t.Fatalf(
			"a comment past the text: err = %v, want ErrForm",
			err,
		)
	}
}
