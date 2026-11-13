package tests

import (
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestEmit_AnEmptyTupleTailRoundTrips proves a body that ends in the empty
// tuple () or ( ) is written whole: "So a flow is the whole script" (the doc of
// Of), so the script Emit writes from a flow Of derived must parse, and derive
// a flow that writes the same script again. An end-of-line comment is trimmed
// by design, so the script is compared, not the first flow.
//
// Revisions:
//   - 2026-11-10 11:53: initial creation (C118, B258)
func TestEmit_AnEmptyTupleTailRoundTrips(t *testing.T) {
	var tests = []struct {
		name string
		text string
	}{
		{
			"assigned in main",
			`
def main():
    x = ()
`,
		},
		{
			"returned from a function",
			`
def g():
    return ()

def main():
    g()
`,
		},
		{
			"with a space inside",
			`
def main():
    x = ( )
`,
		},
		{
			"split over two lines",
			`
def main():
    x = (
    )
`,
		},
		{
			"in a conditional expression",
			`
def main():
    x = 1 if True else ()
`,
		},
		{
			"empty call",
			`
def main():
    print()
`,
		},
		{
			"nested empty tuple",
			`
def main():
    x = ((), ())
`,
		},
		{
			"before a comment",
			`
def main():
    x = ()  # none
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

				again, err := graph.Of(&common.Source{
					Entry: "z.star",
					Text:  out,
				})
				if err != nil {
					t.Fatalf(
						"the emitted script does not parse: %v\n%s",
						err,
						out,
					)
				}

				back, err := graph.Emit(again)
				if err != nil {
					t.Fatalf(
						"emit again: %v",
						err,
					)
				}

				if string(back) != string(out) {
					t.Fatalf(
						"not stable\n--first\n%s\n--again\n%s",
						out,
						back,
					)
				}
			},
		)
	}
}
