package graph

import (
	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// Assigned is a call bound to a name, and whether it models: a spawn's
// binding, a join's result, or a call's result.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to statement_read.go
//   - 2026-11-06 22:02: a nil assignment does not model (C100c, B155)
//   - 2026-11-10 00:25: renamed from _Assigned, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Assigned(
	stmt *syntax.AssignStmt,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	if stmt == nil || stmt.Op != syntax.EQ {
		return nil, false
	}

	name, ok := stmt.LHS.(*syntax.Ident)
	if !ok {
		return nil, false
	}

	call, ok := stmt.RHS.(*syntax.CallExpr)
	if !ok {
		return nil, false
	}

	return r._Called(
		call,
		name.Name,
		scope,
	)
}

// Invoked is a call of a listed function, bound to its result when the
// script captured one.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to statement_read.go
//   - 2026-11-10 00:25: renamed from _Invoked, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Invoked(
	call *syntax.CallExpr,
	bound string,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	held, ok := r.Call(
		call,
		scope,
	)
	if !ok {
		return nil, false
	}

	held.Result = bound

	if bound != "" {
		scope.Bind(bound)
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Call{
			Call: held,
		},
	}, true
}

// One direction of one subject lives here: reading a function's lines as the
// statements a flow carries. The other direction is in statement_write.go.

// _Modelled is a def's statements, and whether every line of it models.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to statement_read.go
//   - 2026-11-10 01:00: now uses Reading, were _Reading (C120a, Q314)
func (r *Reading) _Modelled(
	def *syntax.DefStmt,
	params []string,
) ([]*workflowpb.Statement, bool) {
	return r._List(
		def.Body,
		r._Opened(
			def,
			params,
		),
	)
}

// _List is one statement list as the flow carries it, and whether every line
// models.
//
// One line the walk cannot model makes the whole list fail, and the function
// holding it keeps its text: there is no hole. The closing pass is the
// generator's and is not a statement. A match is two lines - the assignment
// to _match and the chain comparing it - and is read as one statement.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to statement_read.go
//   - 2026-11-10 01:00: now uses Reading, Scope, were _Reading, _Scope (C120a, Q314)
func (r *Reading) _List(
	body []syntax.Stmt,
	scope *Scope,
) ([]*workflowpb.Statement, bool) {
	body = dialect.Closed(body)

	var stmts []*workflowpb.Statement

	for idx := 0; idx < len(body); idx++ {
		match := r._Matched(
			body,
			idx,
			scope,
		)
		if match != nil {
			stmts = append(
				stmts,
				match,
			)
			idx++

			continue
		}

		stmt, ok := r._Statement(
			body[idx],
			scope,
		)
		if !ok {
			return nil, false
		}

		stmts = append(
			stmts,
			stmt,
		)
	}

	return stmts, len(stmts) > 0
}

// _Statement is one line as the flow carries it, and whether it models.
//
// A call, alone or bound to a name, an if, or a for. Anything else - a return,
// a pass that is not a closer, an assignment that is not a call - is a line the
// schema cannot say.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 01:32: says whether the statement became steps whole, which is
//     what decides between a generated body and an authored one
//   - 2026-09-30 00:50: an assignment is whole only when what it assigns was,
//     so a spawn whose arguments could not be carried keeps its function's text
//   - 2026-10-02 00:04: one Statement or none, read in a scope
//   - 2026-11-02 10:25: moved to statement_read.go
//   - 2026-11-10 01:00: now uses Reading, Scope, Assigned, If, and 1 more, were _Reading, _Scope,
//     _Assigned, _If, and 1 more (C120a, Q314)
func (r *Reading) _Statement(
	stmt syntax.Stmt,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	switch actual := stmt.(type) {
	case *syntax.ExprStmt:
		call, ok := actual.X.(*syntax.CallExpr)
		if !ok {
			return nil, false
		}

		return r._Called(
			call,
			"",
			scope,
		)

	case *syntax.AssignStmt:
		return r.Assigned(
			actual,
			scope,
		)

	case *syntax.IfStmt:
		return r.If(
			actual,
			scope,
		)

	case *syntax.ForStmt:
		return r.Loop(
			actual,
			scope,
		)
	}

	return nil, false
}

// _Called is the statement a call is, bound to bound when the script captured
// one, and whether it models.
//
// A call of a listed function is a Call. A builtin this runtime owns is its own
// statement. Anything else - a library function, a builtin of the interpreter,
// a function the script binds to a parameter - is not one.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to statement_read.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: now uses Reading, Scope, Callable, Invoked, and 1 more, were _Reading,
//     _Scope, _Callable, _Invoked, and 1 more (C120a, Q314)
func (r *Reading) _Called(
	call *syntax.CallExpr,
	bound string,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	name := _Callee(call)

	if scope.Callable(name) {
		return r.Invoked(
			call,
			bound,
			scope,
		)
	}

	if !scope.Usable(name) {
		return nil, false
	}

	switch name {
	case common.SPAWN:
		return r._Spawn(
			call,
			bound,
			scope,
		)

	case common.JOIN:
		return r._Join(
			call,
			bound,
			scope,
		)
	}

	return r._Known(
		name,
		call,
		bound,
		scope,
	)
}

// _Known is a call of a name the dialect knows, when it is not bound.
//
// Nothing else a builtin returns is a value a flow binds, so a bound call
// of one of these is left for Starlark.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:25: moved to statement_read.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: now uses Reading, Scope, Cancel, Sleep, and 1 more, were _Reading, _Scope,
//     _Cancel, _Sleep, and 1 more (C120a, Q314)
func (r *Reading) _Known(
	name string,
	call *syntax.CallExpr,
	bound string,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	if bound != "" {
		return nil, false
	}

	switch name {
	case common.CANCEL:
		return r.Cancel(
			call,
			scope,
		)

	case common.SLEEP:
		return r.Sleep(call)

	case common.REPEAT:
		fallthrough
	case common.RETRY:
		return r._Attempts(
			name,
			call,
			scope,
		)

	case common.TIMEOUT:
		return r.Timeout(
			call,
			scope,
		)
	}

	return nil, false
}
