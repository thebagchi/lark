package graph

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// ErrNotCarried is returned for an arg() call a graph cannot carry: a name
// that is not a string literal, a default no value describes, or a count of
// arguments the builtin does not take.
//
// An error rather than a fallthrough to ErrConstant. Once arg is the name a
// declaration is written with, a malformed one is a broken declaration and not
// an ordinary constant that happens to fail - and a reader told "constant
// cannot be carried" would go looking for the wrong thing.
var ErrNotCarried = errors.New("argument declaration cannot be carried")

const (
	// ARG is the builtin a declaration is written with.
	ARG = "arg"

	// TAKES is the most a declaration passes: the name, and a default.
	TAKES = 2
)

// _Declared is the argument an expression declares, or nothing when the
// expression declares none.
//
// Three outcomes, because a module-level statement is one of three things and
// the caller acts differently on each: an arg() call this can carry, an arg()
// call it cannot, and anything else - which is some other kind of constant and
// this one's business is finished.
//
// A script defining a function of its own called arg is reading that function,
// not the builtin, because a global shadows a predeclared name - so its calls
// are left to be carried as the ordinary constants they are. Without that a
// script this runtime runs is a script no graph can describe.
//
// A keyword default is read as a positional one. arg("host", default = "x")
// and arg("host", "x") declare the same argument, and a graph that refused the
// first would refuse a script that runs.
//
// Returns ErrNotCarried for the second outcome.
//
// Revisions:
//   - 2026-09-22 22:31: initial creation
//   - 2026-09-22 23:12: leaves a script's own arg alone, which shadows the
//     builtin when a script defines one
func (r *_Reading) _Declared(expr syntax.Expr) (*workflowpb.Arg, error) {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return nil, nil
	}

	if _Bare(call) != ARG || r.defs[ARG] != nil {
		return nil, nil
	}

	if len(call.Args) == 0 || len(call.Args) > TAKES {
		return nil, fmt.Errorf("%s takes one or two arguments: %w", ARG, ErrNotCarried)
	}

	name, ok := _Spelled(call.Args[0])
	if !ok {
		return nil, fmt.Errorf("%s names a string: %w", ARG, ErrNotCarried)
	}

	if len(call.Args) == 1 {
		return &workflowpb.Arg{Name: name}, nil
	}

	// Not prefixed with the name: the caller already names the binding this
	// declaration belongs to, and for the usual script the two are the same
	// word printed twice.
	value, ok := _Arg(_Passed(call.Args[1]))
	if !ok {
		return nil, fmt.Errorf("default is not a value: %w", ErrNotCarried)
	}

	return &workflowpb.Arg{Name: name, Default: value}, nil
}

// _Passed is what an argument states, with a keyword's name set aside.
//
// Revisions:
//   - 2026-09-22 22:31: initial creation
func _Passed(expr syntax.Expr) syntax.Expr {
	keyword, ok := expr.(*syntax.BinaryExpr)
	if !ok || keyword.Op != syntax.EQ {
		return expr
	}

	return keyword.Y
}

// _Spelled is the string a literal states.
//
// Not _Arg, which would take a bare word as a string too. A declaration names
// itself with a literal, and arg(host) naming the argument "host" would read
// as a script passing a variable and getting away with it.
//
// Revisions:
//   - 2026-09-22 22:31: initial creation
func _Spelled(expr syntax.Expr) (string, bool) {
	literal, ok := expr.(*syntax.Literal)
	if !ok {
		return "", false
	}

	word, ok := literal.Value.(string)

	return word, ok
}
