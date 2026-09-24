package state

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"
)

// ErrNotData is returned for something a store cannot usefully hold.
//
// A store exists so that threads pass data to each other. A function is code,
// and a frozen one read back by another thread is the same object the script
// already had; a handle names a thread, and a thread means nothing to whoever
// did not start it. Storing either is a mistake that reads as if it worked -
// the value goes in, comes back out, and does nothing.
var ErrNotData = errors.New("not data a store can hold")

// Check refuses a set of something the source already shows is not data.
//
// Only what is visible. state.set("k", helper) names a function this file
// declares, and a lambda is one written in place - both are certain before
// anything runs, and a script author would rather hear it then. Everything
// else is caught when it runs, by the same rule: a call's result, a value
// read from somewhere, a function an update returns.
//
// This is a Checking, which the compiler asks every plugin for. The compiler
// does not know what set means, and does not have to.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
func (s *_State) Check(tree *syntax.File) error {
	declared := map[string]bool{}

	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok {
			declared[def.Name.Name] = true
		}
	}

	// A file that binds state itself means its own thing by that name, since
	// a global shadows a predeclared one. Refusing its calls would refuse a
	// script this runtime runs, and blame a store it never reached.
	if _Shadowed(tree) {
		return nil
	}

	var failure error

	syntax.Walk(tree, func(node syntax.Node) bool {
		if failure != nil {
			return false
		}

		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) != 2 || !_Stores(call) {
			return true
		}

		named := _Names(call.Args[1], declared)
		if named == "" {
			return true
		}

		failure = fmt.Errorf("%s.%s at %s stores %s: %w",
			NAME, SET, call.Lparen, named, ErrNotData)

		return false
	})

	return failure
}

// _Shadowed reports whether this file binds the module's own name.
//
// A def or an assignment at the top level, which is where a global is bound.
// Anything deeper is a local and cannot reach the calls this walks.
//
// Revisions:
//   - 2026-09-24 20:34: initial creation
func _Shadowed(tree *syntax.File) bool {
	for _, stmt := range tree.Stmts {
		switch held := stmt.(type) {
		case *syntax.DefStmt:
			if held.Name.Name == NAME {
				return true
			}

		case *syntax.AssignStmt:
			bound, ok := held.LHS.(*syntax.Ident)
			if ok && bound.Name == NAME {
				return true
			}
		}
	}

	return false
}

// _Stores reports whether a call is state.set.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
func _Stores(call *syntax.CallExpr) bool {
	member, ok := call.Fn.(*syntax.DotExpr)
	if !ok || member.Name.Name != SET {
		return false
	}

	module, ok := member.X.(*syntax.Ident)

	return ok && module.Name == NAME
}

// _Names is what the second argument plainly is, when that is a function, and
// empty when the source does not say.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
func _Names(arg syntax.Expr, declared map[string]bool) string {
	switch held := arg.(type) {
	case *syntax.LambdaExpr:
		return "a lambda"

	case *syntax.Ident:
		if declared[held.Name] {
			return "the function " + held.Name
		}
	}

	return ""
}
