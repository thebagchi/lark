package tests

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
)

// TestCheck_RefusesATextThatIsNoStarlark proves Check parses every text a flow
// carries, and refuses what does not parse with ErrForm (B295, Q449): the text of
// main, a function body (a suite), and an init text (statements at the margin).
// Read and Emit refuse it with the same words, since both run Check.
//
// The check: go test -race -count=1 -run TestCheck_RefusesATextThatIsNoStarlark
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 19:29: initial creation (C133, B295, Q449)
func TestCheck_RefusesATextThatIsNoStarlark(t *testing.T) {
	for _, tc := range UNPARSED {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var flow = new(workflowpb.Flow)

				err := protojson.Unmarshal(
					[]byte(tc.flow),
					flow,
				)
				if err != nil {
					t.Fatalf(
						"the flow must decode: %v",
						err,
					)
				}

				checked := runtime.Check(flow)
				if checked == nil {
					t.Fatal("Check must refuse the flow")
				}

				if !errors.Is(
					checked,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						checked,
						runtime.ErrForm,
					)
				}

				if tc.want != "" && checked.Error() != tc.want {
					t.Fatalf(
						"got %q, want %q",
						checked,
						tc.want,
					)
				}

				if !strings.Contains(
					checked.Error(),
					"is not Starlark",
				) {
					t.Fatalf(
						"%q does not say the body is not Starlark",
						checked,
					)
				}

				if len(checked.Error()) > BOUND {
					t.Fatalf(
						"the error is %d bytes, want at most %d",
						len(checked.Error()),
						BOUND,
					)
				}

				_, emitted := runtime.Emit(flow)
				if emitted == nil || emitted.Error() != checked.Error() {
					t.Fatalf(
						"Emit says %v, Check says %v",
						emitted,
						checked,
					)
				}

				_, read := runtime.Read([]byte(tc.flow))
				if read == nil || read.Error() != checked.Error() {
					t.Fatalf(
						"Read says %v, Check says %v",
						read,
						checked,
					)
				}
			},
		)
	}
}

// TestCheck_AcceptsATextThatParses proves a text that parses is no reason to
// refuse: a string over several lines in a body and in init, a comment, and a
// statement that goes over several lines.
//
// The check: go test -race -count=1 -run TestCheck_AcceptsATextThatParses
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 19:29: initial creation (C133, B295, Q449)
func TestCheck_AcceptsATextThatParses(t *testing.T) {
	for _, tc := range PARSED {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := runtime.Read([]byte(tc.flow))
				if err != nil {
					t.Fatal(err)
				}

				_, err = runtime.Emit(flow)
				if err != nil {
					t.Fatalf(
						"Emit refuses a flow Check accepts: %v",
						err,
					)
				}
			},
		)
	}
}

// BEGIN GOLDEN: no test writes these tables; edit by hand only to review.

// UNPARSED lists a flow with a text that is no Starlark, and the words of Check when
// they can be given before the fix; an empty word is only a pattern.
//
// Revisions:
//   - 2026-11-13 19:29: initial creation (C133, B295, Q449)
//
//nolint:lll // a golden row
var (
	UNPARSED = []struct{ name, flow, want string }{
		{
			`the text of main`,
			`{"text": "!"}`,
			`main: body is not Starlark (line 1: unexpected input character '!'): no form`,
		},
		{
			`a function body`,
			`{"functions": [{"name": "f", "body": "x ="}], "main": {"statement": [{"call": {"function": "f"}}]}}`,
			`"f": body is not Starlark (line 1: got newline, want primary expression): no form`,
		},
		{
			`an init text`,
			`{"init": [{"text": "x ="}], "text": "pass"}`,
			`init 1: text is not Starlark (line 1: got newline, want primary expression): no form`,
		},
		{
			`the second init text, which is the one that is bad`,
			`{"init": [{"text": "x = 1"}, {"text": "!"}], "text": "pass"}`,
			`init 2: text is not Starlark (line 1: unexpected input character '!'): no form`,
		},
		{
			`an init text with an indent, which a suite would take`,
			`{"init": [{"text": "  x = 1"}], "text": "pass"}`,
			`init 1: text is not Starlark (line 1: got indent, want primary expression): no form`,
		},
		{
			`an init text with its second line bad`,
			`{"init": [{"text": "x = 1\n!"}], "text": "pass"}`,
			`init 1: text is not Starlark (line 2: unexpected input character '!'): no form`,
		},
		{
			`a block with no indent`,
			`{"text": "if True:\npass"}`,
			``,
		},
		{
			`a second line that is no Starlark`,
			`{"text": "pass\n)"}`,
			``,
		},
		{
			`brackets that never close, 5,000 of them`,
			`{"text": "` + strings.Repeat(
				"(",
				5000,
			) + `"}`,
			``,
		},
		{
			`a long line of garbage`,
			`{"text": "x = ` + strings.Repeat(
				"$",
				5000,
			) + `"}`,
			``,
		},
	}
)

// PARSED lists a flow with a text that parses.
//
// Revisions:
//   - 2026-11-13 19:29: initial creation (C133, B295, Q449)
//
//nolint:lll // a golden row
var (
	PARSED = []struct{ name, flow string }{
		{
			`a string over several lines in a function body`,
			`{"functions": [{"name": "f", "body": "x = \"\"\"a\nb\"\"\"\nreturn x"}], "main": {"statement": [{"call": {"function": "f"}}]}}`,
		},
		{
			`a string over several lines in init`,
			`{"init": [{"text": "X = \"\"\"a\nb\"\"\""}], "text": "pass"}`,
		},
		{
			`a statement over several lines in init`,
			`{"init": [{"text": "X = [\n  1,\n  2,\n]"}], "text": "pass"}`,
		},
		{
			`a comment after a statement`,
			`{"text": "pass  # done"}`,
		},
	}
)

// END GOLDEN
