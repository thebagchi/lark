package graph

import (
	"strings"

	"go.starlark.net/syntax"
)

// _Name is an identifier.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Name(name string) *syntax.Ident {
	return &syntax.Ident{Name: name}
}

// _Text is a string literal, quotes included, as Starlark spells it.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Text(raw string) *syntax.Literal {
	return &syntax.Literal{Token: syntax.STRING, Raw: raw}
}

// _Num is a number literal. A fraction is a float and a whole number is an
// int, which is the spelling _Number already chose.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Num(raw string) *syntax.Literal {
	token := syntax.INT
	if strings.Contains(raw, ".") {
		token = syntax.FLOAT
	}

	return &syntax.Literal{Token: token, Raw: raw}
}

// _Bare is a call of a named function.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Bare(fn string, args []syntax.Expr) *syntax.CallExpr {
	return &syntax.CallExpr{Fn: _Name(fn), Args: args}
}

// _Lambda is a lambda with no parameters. The flow does not store any.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Lambda(body syntax.Expr) *syntax.LambdaExpr {
	return &syntax.LambdaExpr{Body: body}
}

// _Emit is an expression, bound to name when the script captured one.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Emit(name string, expr syntax.Expr) syntax.Stmt {
	if name == "" {
		return &syntax.ExprStmt{X: expr}
	}

	return &syntax.AssignStmt{Op: syntax.EQ, LHS: _Name(name), RHS: expr}
}

// _Def is a function. body is its statements, with no closer yet.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Def(name string, params []string, body []syntax.Stmt) *syntax.DefStmt {
	list := make([]syntax.Expr, 0, len(params))

	for _, param := range params {
		list = append(list, _Name(param))
	}

	return &syntax.DefStmt{Name: _Name(name), Params: list, Body: body}
}

// _Pass is the closer a suite gains when it does not return. The flow does
// not store it.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Pass() *syntax.BranchStmt {
	return &syntax.BranchStmt{Token: syntax.PASS}
}
