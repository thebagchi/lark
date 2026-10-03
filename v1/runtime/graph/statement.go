package graph

import (
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// Both directions of one subject live here: reading a function's lines as the
// statements a flow carries, and writing a flow's statements back as lines.

// _Modelled is a def's statements, and whether every line of it models.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Modelled(
	def *syntax.DefStmt,
	params []string,
) ([]*workflowpb.Statement, bool) {
	return r._List(def.Body, r._Opened(def, params))
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
func (r *_Reading) _List(body []syntax.Stmt, scope *_Scope) ([]*workflowpb.Statement, bool) {
	body = dialect.Closed(body)

	var stmts []*workflowpb.Statement

	for idx := 0; idx < len(body); idx++ {
		match := r._Matched(body, idx, scope)
		if match != nil {
			stmts = append(stmts, match)
			idx++

			continue
		}

		stmt, ok := r._Statement(body[idx], scope)
		if !ok {
			return nil, false
		}

		stmts = append(stmts, stmt)
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
func (r *_Reading) _Statement(stmt syntax.Stmt, scope *_Scope) (*workflowpb.Statement, bool) {
	switch actual := stmt.(type) {
	case *syntax.ExprStmt:
		call, ok := actual.X.(*syntax.CallExpr)
		if !ok {
			return nil, false
		}

		return r._Called(call, "", scope)

	case *syntax.AssignStmt:
		return r._Assigned(actual, scope)

	case *syntax.IfStmt:
		return r._If(actual, scope)

	case *syntax.ForStmt:
		return r._Loop(actual, scope)
	}

	return nil, false
}

// _Assigned is a call bound to a name, and whether it models: a spawn's
// binding, a join's result, or a call's result.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Assigned(
	stmt *syntax.AssignStmt,
	scope *_Scope,
) (*workflowpb.Statement, bool) {
	if stmt.Op != syntax.EQ {
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

	return r._Called(call, name.Name, scope)
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
func (r *_Reading) _Called(
	call *syntax.CallExpr,
	bound string,
	scope *_Scope,
) (*workflowpb.Statement, bool) {
	name := _Callee(call)

	if scope._Callable(name) {
		return r._Invoked(call, bound, scope)
	}

	if !scope._Usable(name) {
		return nil, false
	}

	switch name {
	case SPAWN:
		return r._Spawn(call, bound, scope)

	case JOIN:
		return r._Join(call, bound, scope)
	}

	// Nothing else a builtin returns is a value a flow binds.
	if bound != "" {
		return nil, false
	}

	switch name {
	case CANCEL:
		return r._Cancel(call, scope)

	case SLEEP:
		return r._Sleep(call)

	case REPEAT:
		fallthrough
	case RETRY:
		return r._Attempts(name, call, scope)

	case TIMEOUT:
		return r._Timeout(call, scope)
	}

	return nil, false
}

// _Invoked is a call of a listed function, bound to its result when the
// script captured one.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Invoked(
	call *syntax.CallExpr,
	bound string,
	scope *_Scope,
) (*workflowpb.Statement, bool) {
	held, ok := r._Call(call, scope)
	if !ok {
		return nil, false
	}

	held.Result = bound

	if bound != "" {
		scope._Bind(bound)
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Call{Call: held}}, true
}

// _Code is a function's body.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Function
func _Code(prog *_Program, fn *workflowpb.Function) ([]syntax.Stmt, error) {
	switch held := fn.GetCode().(type) {
	case *workflowpb.Function_Body:
		return _Parsed(prog, fn.GetName(), held.Body)

	case *workflowpb.Function_Statements:
		return _ListBody(fn.GetName(), held.Statements.GetStatement(), prog)

	default:
		return nil, fmt.Errorf("%s: %w", fn.GetName(), ERR_NO_BODY)
	}
}

// _Entry is main's body.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Flow
func _Entry(prog *_Program, flow *workflowpb.Flow) ([]syntax.Stmt, error) {
	switch held := flow.GetSpine().(type) {
	case *workflowpb.Flow_Text:
		return _Parsed(prog, ENTRY, held.Text)

	case *workflowpb.Flow_Main:
		return _ListBody(ENTRY, held.Main.GetStatement(), prog)

	default:
		return nil, fmt.Errorf("%s: %w", ENTRY, ERR_NO_BODY)
	}
}

// _ListBody is a statement list. An empty list is not a def.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Statement
func _ListBody(name string, list []*workflowpb.Statement, prog *_Program) ([]syntax.Stmt, error) {
	if len(list) == 0 {
		return nil, fmt.Errorf("%s: %w", name, ERR_NO_BODY)
	}

	return _Statements(list, prog)
}

// _Statements is a list of statements. A match is more than one.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Statement
func _Statements(list []*workflowpb.Statement, prog *_Program) ([]syntax.Stmt, error) {
	var out []syntax.Stmt

	for _, stmt := range list {
		part, err := _Statement(stmt, prog)
		if err != nil {
			return nil, err
		}

		out = append(out, part...)
	}

	return out, nil
}

// _Statement is one statement. A match is the assignment and the comparisons.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Statement
func _Statement(stmt *workflowpb.Statement, prog *_Program) ([]syntax.Stmt, error) {
	if stmt == nil || stmt.GetAction() == nil {
		return nil, fmt.Errorf("statement: %w", ERR_FORM)
	}

	switch stmt.GetAction().(type) {
	case *workflowpb.Statement_Call:
		return _Single(_Call(stmt.GetCall()))

	case *workflowpb.Statement_Spawn:
		return _Single(_Spawn(stmt.GetSpawn()))

	case *workflowpb.Statement_Join:
		return _Single(_Join(stmt.GetJoin()))

	case *workflowpb.Statement_Cancel:
		return _Single(_Cancel(stmt.GetCancel()))

	case *workflowpb.Statement_Sleep:
		return _Single(_Sleep(stmt.GetSleep()))

	case *workflowpb.Statement_Repeat:
		return _Single(_Repeat(stmt.GetRepeat()))

	case *workflowpb.Statement_Retry:
		return _Single(_Retry(stmt.GetRetry()))

	case *workflowpb.Statement_Timeout:
		return _Single(_Timeout(stmt.GetTimeout()))

	case *workflowpb.Statement_Loop:
		return _Single(_Loop(stmt.GetLoop(), prog))

	case *workflowpb.Statement_If:
		return _Single(_If(stmt.GetIf(), false, prog))

	case *workflowpb.Statement_Match:
		return _Match(stmt.GetMatch(), prog)

	default:
		return nil, fmt.Errorf("statement: %w", ERR_FORM)
	}
}

// _Single is one statement as a list, so a match and a call share a caller.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Single(stmt syntax.Stmt, err error) ([]syntax.Stmt, error) {
	if err != nil {
		return nil, err
	}

	return []syntax.Stmt{stmt}, nil
}
