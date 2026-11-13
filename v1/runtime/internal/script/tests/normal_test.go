package tests

import (
	"bytes"
	"slices"
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
)

// TestNormal_WritesOnlyALoneCarriageReturnAsALineFeed proves Normal changes a
// carriage return that no line feed follows, and nothing else: a carriage return
// and a line feed together stay, so a file with those line breaks is the same
// bytes, and so does a NUL. A text with no lone carriage return is returned as
// it is, not copied (Q405).
//
// The check: go test -count=1 -run TestNormal_WritesOnlyALoneCarriageReturnAsALineFeed
// ./v1/runtime/internal/script/tests
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func TestNormal_WritesOnlyALoneCarriageReturnAsALineFeed(t *testing.T) {
	var tests = []struct {
		name string
		text string
		want string
	}{
		{
			"empty",
			"",
			"",
		},
		{
			"no carriage return",
			"a\nb\n",
			"a\nb\n",
		},
		{
			"a lone return",
			"a\rb\r",
			"a\nb\n",
		},
		{
			"a return and a line feed",
			"a\r\nb\r\n",
			"a\r\nb\r\n",
		},
		{
			"both kinds",
			"a\rb\r\nc\r",
			"a\nb\r\nc\n",
		},
		{
			"two returns, then a line feed",
			"a\r\r\nb",
			"a\n\r\nb",
		},
		{
			"a return at the end",
			"a\r",
			"a\n",
		},
		{
			"a NUL stays",
			"a\x00\rb",
			"a\x00\nb",
		},
		{
			"a return in a string over lines",
			"'''a\rb'''",
			"'''a\nb'''",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					in  = []byte(tc.text)
					got = script.Normal(in)
				)

				if string(got) != tc.want {
					t.Fatalf(
						"normal: got %q, want %q",
						got,
						tc.want,
					)
				}

				if string(in) != tc.text {
					t.Fatalf(
						"the text given was changed: %q",
						in,
					)
				}

				if tc.text == tc.want && len(in) > 0 && &got[0] != &in[0] {
					t.Fatal("a text with no lone return was copied")
				}
			},
		)
	}
}

// TestNormal_KeepsEveryLineAndStringValue proves what Q405 rests on: the parser
// reads a lone carriage return as a line break, so the script after Normal has
// the same tokens, on the same lines and columns, with the same string and bytes
// values, including a string over several lines that holds a lone return, and
// the same comments.
//
// The check: go test -count=1 -run TestNormal_KeepsEveryLineAndStringValue
// ./v1/runtime/internal/script/tests
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func TestNormal_KeepsEveryLineAndStringValue(t *testing.T) {
	var tests = []string{
		"x = 1\ry = 2\rdef main():\r    print(x, y)\r",
		"s = '''a\rb\r  c'''\ndef main():\n    print(s)\n",
		"s = '''a\rb'''\rb = b'''x\ry'''\rr = r'''p\rq'''\r",
		"s = 'a\\\rb'\rdef main():\r    pass\r",
		"x = [1,\r  2]  # c\r# own\rdef main():\r    f(\r#a\r1)\r",
		"def main():\r\r\r    x = 1 # c\r    return x\r",
		"a = 1\r\nb = '''x\ry\r\nz'''\r",
	}

	for _, text := range tests {
		t.Run(
			text,
			func(t *testing.T) {
				var (
					before = _Tokens(
						t,
						text,
					)
					after = _Tokens(
						t,
						string(script.Normal([]byte(text))),
					)
				)

				if !slices.Equal(
					before,
					after,
				) {
					t.Fatalf(
						"tokens differ:\nbefore %q\nafter  %q",
						before,
						after,
					)
				}
			},
		)
	}
}

// _Tokens is every literal and identifier of text with its line and column, its
// value, and every comment, as strings.
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func _Tokens(
	t *testing.T,
	text string,
) []string {
	t.Helper()

	file, err := (&syntax.FileOptions{}).Parse(
		"normal.star",
		text,
		syntax.RetainComments,
	)
	if err != nil {
		t.Fatalf(
			"parse: %v",
			err,
		)
	}

	var found []string

	syntax.Walk(
		file,
		func(node syntax.Node) bool {
			if node == nil {
				return false
			}

			start, end := node.Span()

			switch held := node.(type) {
			case *syntax.Literal:
				found = append(
					found,
					"lit",
					held.Raw,
					start.String(),
					end.String(),
				)

				if value, ok := held.Value.(string); ok {
					found = append(
						found,
						"value:"+value,
					)
				}

			case *syntax.Ident:
				found = append(
					found,
					"id:"+held.Name,
					start.String(),
				)
			}

			if notes := node.Comments(); notes != nil {
				for _, list := range [][]syntax.Comment{
					notes.Before,
					notes.Suffix,
					notes.After,
				} {
					for _, note := range list {
						found = append(
							found,
							"note:"+note.Text,
							note.Start.String(),
						)
					}
				}
			}

			return true
		},
	)

	return found
}

// TestNormalised_ReadsEveryModuleThroughNormal proves a source after Normalised
// has a normal entry text and a loader that gives every module normal, and that
// the source given is not changed; a nil source stays nil.
//
// The check: go test -count=1 -run TestNormalised_ReadsEveryModuleThroughNormal
// ./v1/runtime/internal/script/tests
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func TestNormalised_ReadsEveryModuleThroughNormal(t *testing.T) {
	if script.Normalised(nil) != nil {
		t.Fatal("a nil source did not stay nil")
	}

	var (
		loader = &_Modules{
			text: "m = 1\r",
		}
		source = &common.Source{
			Entry:  "main.star",
			Text:   []byte("x = 1\r"),
			Loader: loader,
		}
		got = script.Normalised(source)
	)

	if string(got.Text) != "x = 1\n" || got.Entry != "main.star" {
		t.Fatalf(
			"entry: got %q %q",
			got.Entry,
			got.Text,
		)
	}

	if string(source.Text) != "x = 1\r" {
		t.Fatalf(
			"the source given was changed: %q",
			source.Text,
		)
	}

	name, err := got.Loader.Resolve(
		"main.star",
		"m.star",
	)
	if err != nil || name != "m.star/resolved" {
		t.Fatalf(
			"resolve: got %q, %v",
			name,
			err,
		)
	}

	text, err := got.Loader.Load("m.star")
	if err != nil || !bytes.Equal(
		text,
		[]byte("m = 1\n"),
	) {
		t.Fatalf(
			"load: got %q, %v",
			text,
			err,
		)
	}

	_, err = script.Normalised(&common.Source{
		Loader: &_Modules{},
	}).Loader.Load("")
	if err == nil {
		t.Fatal("a load error was not passed on")
	}
}

// _Modules is a Loader that serves one text for every name, or fails with none.
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
type _Modules struct {
	text string
}

// Resolve gives target a suffix, so a test sees it was asked.
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func (m *_Modules) Resolve(
	from string,
	target string,
) (string, error) {
	return target + "/resolved", nil
}

// Load gives the text, or an error when there is none.
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func (m *_Modules) Load(name string) ([]byte, error) {
	if m.text == "" {
		return nil, bytes.ErrTooLarge
	}

	return []byte(m.text), nil
}
