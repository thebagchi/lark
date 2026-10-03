package graph

import (
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// Both directions of one subject live here: reading a spawn, a join and a
// cancel out of a script, and writing them back. A flow names a thread by the
// binding the script gave its spawn, never by an id: the run assigns ids, and
// a flow that predicted one would be a guess.

// _Spawn is the statement a spawn is, bound to bound when the script named it,
// and whether it models.
//
// A bare spawn has no binding. A binding this list already gave a spawn is
// refused: the second would hide the first, and a join could not say which.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-30 00:41: the fork carries its call, and an argument that cannot
//     be carried is reported rather than dropped
//   - 2026-10-02 00:04: a Spawn carrying the script's binding rather than a
//     thread id, and no thread read for what it starts
func (r *_Reading) _Spawn(
	call *syntax.CallExpr,
	bound string,
	scope *_Scope,
) (*workflowpb.Statement, bool) {
	if len(call.Args) != 1 {
		return nil, false
	}

	target, ok := r._Target(call.Args[0], scope)
	if !ok {
		return nil, false
	}

	if bound != "" && !scope._Fork(bound) {
		return nil, false
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Spawn{
		Spawn: &workflowpb.Spawn{Binding: bound, Call: target},
	}}, true
}

// _Join is the statement a join is, bound to its result when the script
// captured one, and whether it models.
//
// Every argument names a spawn's binding the list can see. A spawn written
// inside the join names no binding, so join(spawn(work)) keeps its function's
// text.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Join(
	call *syntax.CallExpr,
	bound string,
	scope *_Scope,
) (*workflowpb.Statement, bool) {
	bindings, ok := _Bindings(call.Args, scope)
	if !ok {
		return nil, false
	}

	if bound != "" {
		scope._Bind(bound)
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Join{
		Join: &workflowpb.Join{Bindings: bindings, Result: bound},
	}}, true
}

// _Cancel is the statement a cancel is, and whether it models.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Cancel(call *syntax.CallExpr, scope *_Scope) (*workflowpb.Statement, bool) {
	bindings, ok := _Bindings(call.Args, scope)
	if !ok {
		return nil, false
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Cancel{
		Cancel: &workflowpb.Cancel{Bindings: bindings},
	}}, true
}

// _Bindings is the spawn bindings a join or a cancel names, and whether every
// argument is one.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func _Bindings(args []syntax.Expr, scope *_Scope) ([]string, bool) {
	var bindings []string

	for _, arg := range args {
		name, ok := arg.(*syntax.Ident)
		if !ok || !scope._Spawned(name.Name) {
			return nil, false
		}

		bindings = append(bindings, name.Name)
	}

	return bindings, true
}

// _Spawn is a spawn, bound when the script named it.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Spawn
func _Spawn(spawn *workflowpb.Spawn) (syntax.Stmt, error) {
	if spawn == nil {
		return nil, fmt.Errorf("spawn: %w", ERR_FORM)
	}

	target, err := _Target(spawn.GetCall())
	if err != nil {
		return nil, err
	}

	return _Emit(spawn.GetBinding(), _Bare(SPAWN, []syntax.Expr{target})), nil
}

// _Join is a join, bound when the script captures the value.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Join
func _Join(join *workflowpb.Join) (syntax.Stmt, error) {
	if join == nil {
		return nil, fmt.Errorf("join: %w", ERR_FORM)
	}

	var args []syntax.Expr

	for _, name := range join.GetBindings() {
		args = append(args, _Name(name))
	}

	return _Emit(join.GetResult(), _Bare(JOIN, args)), nil
}

// _Cancel is a cancel.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Cancel
func _Cancel(cancel *workflowpb.Cancel) (syntax.Stmt, error) {
	if cancel == nil {
		return nil, fmt.Errorf("cancel: %w", ERR_FORM)
	}

	var args []syntax.Expr

	for _, name := range cancel.GetBindings() {
		args = append(args, _Name(name))
	}

	return &syntax.ExprStmt{X: _Bare(CANCEL, args)}, nil
}
