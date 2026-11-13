package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestOf_ReadsADeclarationAsARunDoes proves a derivation reads an arg() call
// as internal/declared does, since that is what a run declares with: a call
// with parentheses round the callee declares (Q301), and a bytes literal names
// nothing (B252). The script Emit writes from the parenthesised form is the
// plain call, and it derives to the same flow.
//
// Revisions:
//   - 2026-11-10 16:04: initial creation (C118 fix round 1, B251, B252)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func TestOf_ReadsADeclarationAsARunDoes(t *testing.T) {
	var tests = []struct {
		name string
		text string
		want string
		fail error
	}{
		{
			"parenthesised callee",
			`
h = (arg)("h", 1)

def main():
    print(h)
`,
			`h = arg("h", 1)`,
			nil,
		},
		{
			"doubly parenthesised callee, no default",
			`
h = ((arg))("h")

def main():
    print(h)
`,
			`h = arg("h")`,
			nil,
		},
		{
			"bytes name",
			`
h = arg(b"x", 1)

def main():
    print(h)
`,
			"",
			graph.ErrCarry,
		},
		{
			"bytes name in a parenthesised callee",
			`
h = (arg)(b"x")

def main():
    print(h)
`,
			"",
			graph.ErrCarry,
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
				if tc.fail != nil {
					if !errors.Is(
						err,
						tc.fail,
					) {
						t.Fatalf(
							"derive: got %v, want %v",
							err,
							tc.fail,
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

				if len(flow.GetArgs()) != 1 || len(flow.GetConstants()) != 0 {
					t.Fatalf(
						"got args %v and constants %v, want one argument",
						flow.GetArgs(),
						flow.GetConstants(),
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
						"emitted script lacks %q:\n%s",
						tc.want,
						out,
					)
				}

				_Same(
					t,
					_Derived(
						t,
						string(out),
					),
					flow,
				)
			},
		)
	}
}
