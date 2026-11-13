package artifact

import (
	"go.starlark.net/syntax"
)

// _Calls reports whether evaluating node can call anything: a builtin, a
// plugin, or a function of the script.
//
// A call is the only way a top level reaches the run. Every builtin that reads
// the run - arg, print, spawn - takes the thread, and only a call hands it the
// thread. So a top level with no call produces the same globals on every run.
//
// A def body and a lambda body run when the function runs, not where it is
// written, so a call there does not count. Its defaults run where it is
// written, so a call there does. Everything else at the top level runs there,
// a comprehension and the condition of a loop included.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func _Calls(node syntax.Node) bool {
	found := false

	syntax.Walk(
		node,
		func(next syntax.Node) bool {
			if found {
				return false
			}

			switch next := next.(type) {
			case *syntax.CallExpr:
				found = true

				return false

			case *syntax.DefStmt:
				found = _Defaults(next.Params)

				return false

			case *syntax.LambdaExpr:
				found = _Defaults(next.Params)

				return false

			default:
				return true
			}
		},
	)

	return found
}

// _Defaults reports whether evaluating the defaults of params can call
// anything.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func _Defaults(params []syntax.Expr) bool {
	for _, param := range params {
		if _Calls(param) {
			return true
		}
	}

	return false
}
