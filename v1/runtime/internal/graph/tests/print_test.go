package tests

// This file tests Program.Node against the nodes it refuses: none, or a kind no
// script is written with. Emit and Of only hand it nodes a flow can hold (C65a).
//
// Revisions:
//   - 2026-11-10 00:25: moved to tests/ from print_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)

import (
	"errors"
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestNode_RefusesWhatItDoesNotWrite checks Program.Node fails as ErrForm
// for a pass over no node, which must not be read as a pass over the whole
// file, and for a built node that is none of the kinds a script is written with
// (no flow builds one, so nothing else reaches this). It also pins the error
// text of a failed template pass in the second row. The text names the
// function the template called, and not a method.
//
// Before C35b the text was:
//
//	script: template: starlark:117:16: executing "starlark" at <$GEN.Refuse>:
//	error calling Refuse: *syntax.UnaryExpr: no form
//
// (one line; wrapped here)
//
// The line and column are in SOURCE, so a change to SOURCE above that line moves
// them. The test then fails with the new text to copy in.
//
// Revisions:
//   - 2026-10-02 11:31: initial creation
//   - 2026-10-02 11:31: initial creation
//   - 2026-10-03 20:44: on an empty program, SOURCE being parsed by PARSED rather than
//     held by the program
//   - 2026-10-03 20:44: on an empty program, SOURCE being parsed by PARSED rather than
//     held by the program
//   - 2026-11-02 17:06: initial creation
//   - 2026-11-08 22:48: merges TestNode_RefusesNothingAtAll, TestNode_RefusesAKindItDoesNotWrite
//     and TestNode_PinsTheTextOfAFailedPass into one table (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNode_RefusesWhatItDoesNotWrite(t *testing.T) {
	cases := []struct {
		name string
		node syntax.Node
		text string
	}{
		{
			"no node",
			nil,
			"",
		},
		{
			"a kind no script is written with",
			&syntax.UnaryExpr{
				Op: syntax.MINUS,
				X: &syntax.Ident{
					Name: "x",
				},
			},
			`script: template: starlark:117:12: ` +
				`executing "starlark" at <Refuse $GEN $NODE>: ` +
				`error calling Refuse: *syntax.UnaryExpr: no form`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := new(graph.Program).Node(
					item.node,
					"",
				)
				if !errors.Is(
					err,
					graph.ErrForm,
				) {
					t.Fatalf(
						"got %v, want ErrForm",
						err,
					)
				}

				if item.text != "" && err.Error() != item.text {
					t.Fatalf(
						"got %q, want %q",
						err.Error(),
						item.text,
					)
				}
			},
		)
	}
}
