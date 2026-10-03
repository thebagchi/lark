package graph

import (
	"errors"
	"testing"

	"go.starlark.net/syntax"
)

// TestNode_RefusesNothingAtAll checks a pass over no node fails, rather than
// being read as a pass over the whole file.
//
// Revisions:
//   - 2026-10-02 11:31: initial creation
//   - 2026-10-03 20:44: on an empty program, SOURCE being parsed by PARSED rather than
//     held by the program
func TestNode_RefusesNothingAtAll(t *testing.T) {
	_, err := new(_Program).Node(nil, "")
	if !errors.Is(err, ERR_FORM) {
		t.Fatalf("got %v, want ERR_FORM", err)
	}
}

// TestNode_RefusesAKindItDoesNotWrite checks a built node that is none of the
// kinds a script is written with fails through the template, as ERR_FORM. No
// flow builds one, so nothing else reaches this.
//
// Revisions:
//   - 2026-10-02 11:31: initial creation
//   - 2026-10-03 20:44: on an empty program, SOURCE being parsed by PARSED rather than
//     held by the program
func TestNode_RefusesAKindItDoesNotWrite(t *testing.T) {
	negated := &syntax.UnaryExpr{Op: syntax.MINUS, X: &syntax.Ident{Name: "x"}}

	_, err := new(_Program).Node(negated, "")
	if !errors.Is(err, ERR_FORM) {
		t.Fatalf("got %v, want ERR_FORM", err)
	}
}
