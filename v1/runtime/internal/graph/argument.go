package graph

import (
	"errors"
	"fmt"
	"strconv"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/internal/declared"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// ErrCarry is returned for what a flow cannot carry, and the wrap names
// which: an arg() call that is malformed (a name that is not a string literal
// or is empty, a default no value describes, or a count of arguments the builtin
// does not take), a module-level constant, a def whose signature has a default, a *args
// or a **kwargs, a top-level statement of a loaded module that is no def, no
// load and no assignment, and a load that renames what it binds.
//
// A giving-up and not a classification: there is nowhere for a constant to
// ride along, and dropping it leaves every function that reads it failing
// with undefined. A statement of a loaded module has no place in a flow, which
// has one init, the entry script's. A load that renames means every body in
// that file says one name while the function is another, and reconciling that
// means rewriting bodies, which derivation never does.
//
// A malformed arg() is an error rather than a fallthrough to a constant: once
// arg is the name a declaration is written with, a malformed one is a broken
// declaration, and a reader told "constant cannot be carried" would go
// looking for the wrong thing.
//
// Revisions:
//   - 2026-09-22 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: renamed from ErrNotCarried (C121, Q348)
//   - 2026-11-11 00:31: an empty arg() name cannot be carried (C122, B262)
var (
	ErrCarry = errors.New("cannot be carried")
)

const (
	// ARG is the builtin a declaration is written with.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ARG = "arg"

	// TAKES is the most a declaration passes: the name, and a default.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
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
// An empty name is the second outcome too. A run can be handed a value under
// it, but a flow keeps a name an Arg lacks apart from one it has by refusing
// both, and Emit has no form for a declaration without a name, so a flow Of
// derived with one would be a flow Emit cannot write (B262).
//
// Returns ErrCarry for the second outcome.
//
// Revisions:
//   - 2026-09-22 22:31: initial creation
//   - 2026-09-22 23:12: leaves a script's own arg alone, which shadows the
//     builtin when a script defines one
//   - 2026-11-10 01:00: now uses Reading, Spelled, Passed, were _Reading, _Spelled, _Passed
//     (C120a, Q314)
//   - 2026-11-10 16:04: reads the callee and the name with internal/declared, as a run does,
//     so (arg)("h") declares and arg(b"x") does not (C118 fix round 1, B251, B252)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
//   - 2026-11-11 00:31: an empty name is not carried (C122, B262)
func (r *Reading) _Declared(expr syntax.Expr) (*workflowpb.Arg, error) {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return nil, nil
	}

	callee := declared.Callee(call)
	if callee == nil || callee.Name != ARG || r.defs[ARG] != nil {
		return nil, nil
	}

	if len(call.Args) == 0 || len(call.Args) > TAKES {
		return nil, fmt.Errorf(
			"%s takes one or two arguments: %w",
			ARG,
			ErrCarry,
		)
	}

	name, ok := declared.Spelled(call.Args[0])
	if !ok {
		return nil, fmt.Errorf(
			"%s names a string: %w",
			ARG,
			ErrCarry,
		)
	}

	if name == "" {
		return nil, fmt.Errorf(
			"%s names an empty string (give it a name): %w",
			ARG,
			ErrCarry,
		)
	}

	if len(call.Args) == 1 {
		return &workflowpb.Arg{
			Name: name,
		}, nil
	}

	// Not prefixed with the name: the caller already names the binding this
	// declaration belongs to, and for the usual script the two are the same
	// word printed twice.
	value, ok := _Arg(Passed(call.Args[1]))
	if !ok {
		return nil, fmt.Errorf(
			"default is not a value: %w",
			ErrCarry,
		)
	}

	return &workflowpb.Arg{
		Name:    name,
		Default: value,
	}, nil
}

// Passed is what an argument states, with a keyword's name set aside.
//
// Revisions:
//   - 2026-09-22 22:31: initial creation
//   - 2026-11-08 10:35: a typed nil binary expression is passed as it is (C108, B214)
//   - 2026-11-10 00:25: renamed from _Passed, as a test in tests/ needs it (C120a, Q314)
func Passed(expr syntax.Expr) syntax.Expr {
	keyword, ok := expr.(*syntax.BinaryExpr)
	if !ok || keyword == nil || keyword.Op != syntax.EQ {
		return expr
	}

	return keyword.Y
}

// _Declaration is the call a script declares an argument with.
//
// An argument with no default is written with none. Writing None there would
// turn an argument a run must supply into one that defaults to nothing.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation, as _Declared
//   - 2026-10-01 13:08: returns the call node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Arg, and named
//     apart from the reading's _Declared
func _Declaration(arg *workflowpb.Arg) (syntax.Expr, error) {
	if arg == nil || arg.GetName() == "" {
		return nil, fmt.Errorf(
			"arg: %w",
			ErrForm,
		)
	}

	args := []syntax.Expr{_Text(strconv.Quote(arg.GetName()))}

	if arg.GetDefault() == nil {
		return _Bare(
			ARG,
			args,
		), nil
	}

	value, err := _Value(arg.GetDefault())
	if err != nil {
		return nil, err
	}

	args = append(
		args,
		value,
	)

	return _Bare(
		ARG,
		args,
	), nil
}
