package graph

import (
	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// One direction of one subject lives here: reading a loop out of a script. The
// other direction is in loop_write.go.

// Loop is the statement a for is, and whether it models.
//
// for <name> in range(<count>), whose count is a whole number or a name the
// scope sees, and whose body is one statement once its closer is set aside.
// The body may not read the loop variable, and may bind nothing: a name it
// bound would be one binding on the flow and one thread per iteration on the
// graph, and after the loop it holds only the last.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to loop_read.go
//   - 2026-11-06 22:02: a nil for does not model (C100c, B153)
//   - 2026-11-10 00:25: renamed from _Loop, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Loop(
	stmt *syntax.ForStmt,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	if stmt == nil {
		return nil, false
	}

	variable, ok := stmt.Vars.(*syntax.Ident)
	if !ok {
		return nil, false
	}

	over, ok := stmt.X.(*syntax.CallExpr)
	if !ok || _Callee(over) != RANGE || !scope.Usable(RANGE) || len(over.Args) != 1 {
		return nil, false
	}

	times, ok := r.Times(
		over.Args[0],
		scope,
	)
	if !ok {
		return nil, false
	}

	body := dialect.Closed(stmt.Body)
	if len(body) != 1 || _Reads(
		body[0],
		variable.Name,
	) || _Binds(body[0]) {
		return nil, false
	}

	inner, ok := r._Statement(
		body[0],
		scope.Inner(),
	)
	if !ok {
		return nil, false
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Loop{
			Loop: &workflowpb.Loop{
				Times: times,
				Body:  inner,
			},
		},
	}, true
}

// Times is the count a loop runs, as an Operand: a whole, non-negative number,
// or a name the scope sees.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to loop_read.go
//   - 2026-11-10 00:25: renamed from _Times, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-13 10:16: the count is an int value (C132, Q407, Q421)
func (r *Reading) Times(
	expr syntax.Expr,
	scope *Scope,
) (*workflowpb.Operand, bool) {
	if name, ok := expr.(*syntax.Ident); ok {
		if _, word := _Word(name.Name); word || !scope.Sees(name.Name) {
			return nil, false
		}

		return &workflowpb.Operand{
			Source: &workflowpb.Operand_Name{
				Name: name.Name,
			},
		}, true
	}

	count, ok := _Natural(expr)
	if !ok {
		return nil, false
	}

	value := _Integral(int64(count))

	return &workflowpb.Operand{
		Source: &workflowpb.Operand_Literal{
			Literal: value,
		},
	}, true
}

// _Reads reports whether a statement reads name anywhere inside it.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to loop_read.go
func _Reads(
	stmt syntax.Stmt,
	name string,
) bool {
	found := false

	syntax.Walk(
		stmt,
		func(node syntax.Node) bool {
			ident, ok := node.(*syntax.Ident)
			if ok && ident.Name == name {
				found = true
			}

			return !found
		},
	)

	return found
}

// Binds reports whether a statement binds a name anywhere inside it: an
// assignment, a loop variable or a def.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-10-03 20:51: through dialect.Bound, the one walk of what binds a name
//   - 2026-11-02 10:25: moved to loop_read.go
//   - 2026-11-10 01:00: comment now names Binds, were _Binds (C120a, Q314)
func _Binds(stmt syntax.Stmt) bool {
	bound := make(map[string]bool)

	dialect.Bound(
		[]syntax.Stmt{stmt},
		bound,
	)

	return len(bound) > 0
}
