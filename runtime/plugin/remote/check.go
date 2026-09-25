package remote

import (
	"fmt"

	"go.starlark.net/syntax"
)

// Check refuses a call that hands one of this plugin's names something the
// source already shows cannot cross.
//
// A remote plugin does not answer this and is never asked. It cannot: the
// question is about a Starlark tree, and putting one on the wire would be a
// far larger schema than Register and Ask. It does not need to, either -
// the constraint is the same for every remote plugin, because every one of
// them is reached by marshalling to protobuf. So this is written once, on the
// host, and each attached plugin answers with it.
//
// That is the shape state already has: the compiler catches what the source
// shows, the marshalling catches the rest. The difference is that here one
// implementation covers every remote plugin rather than each writing its own.
//
// Only what is visible. A name this file declares as a function, or a lambda
// written in place, is certain before anything runs. A value read from
// somewhere is not, and is caught by _Sendable when it runs.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func (r *_Remote) Check(tree *syntax.File) error {
	declared := map[string]bool{}

	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok {
			declared[def.Name.Name] = true
		}
	}

	mine := map[string]bool{}

	for _, named := range r.names {
		mine[named] = true
	}

	var failure error

	syntax.Walk(tree, func(node syntax.Node) bool {
		if failure != nil {
			return false
		}

		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}

		called := _Called(call.Fn)
		if !mine[called] {
			return true
		}

		failure = _Handed(called, call, declared)

		return failure == nil
	})

	return failure
}

// _Called is the name a call names, in the spelling a plugin announced.
//
// Empty when the call is of something this cannot name - an expression, or a
// deeper attribute than a module and a member.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Called(fn syntax.Expr) string {
	switch held := _Bare(fn).(type) {
	case *syntax.Ident:
		return held.Name

	case *syntax.DotExpr:
		module, ok := _Bare(held.X).(*syntax.Ident)
		if !ok {
			return ""
		}

		return module.Name + SEPARATOR + held.Name.Name
	}

	return ""
}

// _Handed is the refusal for an argument the source shows is a function, or
// nil when every argument could cross.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Handed(called string, call *syntax.CallExpr, declared map[string]bool) error {
	for index, given := range call.Args {
		named := _Names(given, declared)
		if named == "" {
			continue
		}

		return fmt.Errorf("%s at %s is given %s as argument %d: %w",
			called, call.Lparen, named, index, ErrArgument)
	}

	return nil
}

// _Names is what an argument plainly is, when that is a function, and empty
// when the source does not say.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Names(arg syntax.Expr, declared map[string]bool) string {
	switch held := _Bare(arg).(type) {
	case *syntax.LambdaExpr:
		return "a lambda"

	case *syntax.Ident:
		if declared[held.Name] {
			return "the function " + held.Name
		}
	}

	return ""
}

// _Bare is an expression with its parentheses taken off.
//
// Extra parentheses do not change what was written, so (helper) shows a
// function as plainly as helper does.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Bare(expr syntax.Expr) syntax.Expr {
	held, ok := expr.(*syntax.ParenExpr)
	if !ok {
		return expr
	}

	return _Bare(held.X)
}
