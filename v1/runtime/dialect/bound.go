package dialect

import (
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect/internal/rewrite"
	"github.com/thebagchi/lark/v1/runtime/internal/nilcheck"
)

// Bound records in into every name stmts bind: a def's own name, what an
// assignment or a for assigns, and what a load binds - inside an if, a for or
// a while as well, and not inside a def, which is a scope of its own and binds
// only its name where it stands.
//
// One walk, so what binds a name is decided in one place. A load stands only
// at a file's top level, so a function's body never reaches that case.
//
// A nil into records nothing. A nil statement, a typed nil one, and a def or a
// load name that is missing are skipped, and a body that holds itself is read
// once, so a tree a host builds by hand never panics or overflows here.
//
// Revisions:
//   - 2026-10-03 20:51: initial creation, from two copies of this walk
//   - 2026-11-04 19:30: skips a nil map and a nil node (C87)
//   - 2026-11-08 10:53: reads a body that holds itself once, through _Gather (C108, B224)
func Bound(
	stmts []syntax.Stmt,
	into map[string]bool,
) {
	if into == nil {
		return
	}

	var seen map[any]bool

	_Gather(
		stmts,
		into,
		&seen,
	)
}

// _Gather is Bound for one list, with seen holding every if, for and while
// already read. seen is made when the first one is met, so a list of plain
// statements costs nothing.
//
// Revisions:
//   - 2026-11-08 10:53: initial creation, the walk Bound had (C108, B224)
//   - 2026-11-10 11:32: uses rewrite.Absent and rewrite.Targets, which moved to
//     internal/rewrite (C120b, Q315)
//   - 2026-11-11 00:40: finds a nil statement with nilcheck.Absent (C122, B268)
func _Gather(
	stmts []syntax.Stmt,
	into map[string]bool,
	seen *map[any]bool,
) {
	for _, stmt := range stmts {
		if nilcheck.Absent(stmt) {
			continue
		}

		switch held := stmt.(type) {
		case *syntax.DefStmt:
			if held.Name != nil {
				into[held.Name.Name] = true
			}

		case *syntax.AssignStmt:
			rewrite.Targets(
				held.LHS,
				into,
			)

		case *syntax.LoadStmt:
			for _, named := range held.To {
				if named != nil {
					into[named.Name] = true
				}
			}

		case *syntax.ForStmt, *syntax.WhileStmt, *syntax.IfStmt:
			_Nested(
				stmt,
				into,
				seen,
			)
		}
	}
}

// _Nested is _Gather for an if, a for or a while: the names it binds and those
// of the lists it holds, unless it was read already.
//
// Revisions:
//   - 2026-11-08 10:55: initial creation, the cases _Gather had (C108, B224)
//   - 2026-11-10 11:32: uses rewrite.Seen and rewrite.Targets, which moved to internal/rewrite
//     (C120b, Q315)
func _Nested(
	stmt syntax.Stmt,
	into map[string]bool,
	seen *map[any]bool,
) {
	if rewrite.Seen(
		stmt,
		seen,
	) {
		return
	}

	switch held := stmt.(type) {
	case *syntax.ForStmt:
		rewrite.Targets(
			held.Vars,
			into,
		)
		_Gather(
			held.Body,
			into,
			seen,
		)

	case *syntax.WhileStmt:
		_Gather(
			held.Body,
			into,
			seen,
		)

	case *syntax.IfStmt:
		_Gather(
			held.True,
			into,
			seen,
		)
		_Gather(
			held.False,
			into,
			seen,
		)
	}
}
