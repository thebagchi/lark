package common

import (
	"go.starlark.net/syntax"
)

// Bare is an expression with its parentheses taken off.
//
// Extra parentheses do not change what was written, so (helper) shows a
// function as plainly as helper does. Without this the two spellings were
// answered differently: one refused where it was written, the other carried
// to the run and refused there - for the same mistake, with the same fix, at
// two different moments.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation, from deep and remote (C59, A4a)
func Bare(expr syntax.Expr) syntax.Expr {
	held, ok := expr.(*syntax.ParenExpr)
	if !ok {
		return expr
	}

	return Bare(held.X)
}

// Names is what an argument plainly is, when that is a function, and empty
// when the source does not say. A lambda is "a lambda", and a name in declared
// is "the function NAME".
//
// Revisions:
//   - 2026-11-04 21:49: initial creation, from deep and remote (C59, A4a)
func Names(
	arg syntax.Expr,
	declared map[string]bool,
) string {
	switch held := Bare(arg).(type) {
	case *syntax.LambdaExpr:
		return "a lambda"

	case *syntax.Ident:
		if declared[held.Name] {
			return "the function " + held.Name
		}
	}

	return ""
}
