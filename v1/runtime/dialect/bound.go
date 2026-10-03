package dialect

import (
	"go.starlark.net/syntax"
)

// Bound records in into every name stmts bind: a def's own name, what an
// assignment or a for assigns, and what a load binds - inside an if, a for or
// a while as well, and not inside a def, which is a scope of its own and binds
// only its name where it stands.
//
// One walk, so what binds a name is decided in one place. A load stands only
// at a file's top level, so a function's body never reaches that case.
//
// Revisions:
//   - 2026-10-03 20:51: initial creation, from two copies of this walk
func Bound(stmts []syntax.Stmt, into map[string]bool) {
	for _, stmt := range stmts {
		switch held := stmt.(type) {
		case *syntax.DefStmt:
			into[held.Name.Name] = true

		case *syntax.AssignStmt:
			_Targets(held.LHS, into)

		case *syntax.LoadStmt:
			for _, named := range held.To {
				into[named.Name] = true
			}

		case *syntax.ForStmt:
			_Targets(held.Vars, into)
			Bound(held.Body, into)

		case *syntax.WhileStmt:
			Bound(held.Body, into)

		case *syntax.IfStmt:
			Bound(held.True, into)
			Bound(held.False, into)
		}
	}
}

// _Targets records in into every name an assignment's or a loop's left side
// binds, which may be one name or a shape of them.
//
// Revisions:
//   - 2026-10-03 20:51: initial creation, from two copies of it
func _Targets(expr syntax.Expr, into map[string]bool) {
	switch held := expr.(type) {
	case *syntax.Ident:
		into[held.Name] = true

	case *syntax.TupleExpr:
		for _, part := range held.List {
			_Targets(part, into)
		}

	case *syntax.ListExpr:
		for _, part := range held.List {
			_Targets(part, into)
		}

	case *syntax.ParenExpr:
		_Targets(held.X, into)
	}
}
