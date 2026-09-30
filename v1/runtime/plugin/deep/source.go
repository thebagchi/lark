package deep

import (
	"go.starlark.net/syntax"
)

// ShowsCode is the first call to module.member in tree whose second argument
// the source plainly shows is a function, and what it shows: "the function
// helper" or "a lambda". Answers nil and "" when there is none.
//
// The half of IsData a compiler can answer. Only what is visible counts: a
// name the file declares with def, or a lambda written in place, is certain
// before anything runs, and a script author would rather hear it then.
// Everything else - a call's result, a value read from somewhere - is left to
// IsData when it runs.
//
// Here rather than in each plugin for the reason IsData is: state.set and
// event.post each take a value across threads, and two readings of "the source
// shows a function" would be two sets of cases to keep in step.
//
// Only a call of exactly two arguments. Any other count is refused when it
// runs, for its arity, and that says more than naming the second of them would.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation, as state's Check
//   - 2026-09-30 22:41: a question any plugin can ask of its own call, rather than
//     state's
func ShowsCode(tree *syntax.File, module string, member string) (*syntax.CallExpr, string) {
	declared := map[string]bool{}

	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok {
			declared[def.Name.Name] = true
		}
	}

	var (
		found *syntax.CallExpr
		named string
	)

	syntax.Walk(tree, func(node syntax.Node) bool {
		if found != nil {
			return false
		}

		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) != 2 || !_Calls(call, module, member) {
			return true
		}

		named = _Names(call.Args[1], declared)
		if named != "" {
			found = call
		}

		return found == nil
	})

	return found, named
}

// _Calls reports whether a call is module.member.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation, as state's _Stores
//   - 2026-09-30 22:41: names the call it looks for, rather than state.set
func _Calls(call *syntax.CallExpr, module string, member string) bool {
	dotted, ok := call.Fn.(*syntax.DotExpr)
	if !ok || dotted.Name.Name != member {
		return false
	}

	held, ok := dotted.X.(*syntax.Ident)

	return ok && held.Name == module
}

// _Names is what an argument plainly is, when that is a function, and empty
// when the source does not say.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation, in state
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
// function as plainly as helper does. Without this the two spellings were
// answered differently: one refused where it was written, the other carried
// to the run and refused there - for the same mistake, with the same fix, at
// two different moments.
//
// Revisions:
//   - 2026-09-24 21:10: initial creation, in state
func _Bare(expr syntax.Expr) syntax.Expr {
	held, ok := expr.(*syntax.ParenExpr)
	if !ok {
		return expr
	}

	return _Bare(held.X)
}
