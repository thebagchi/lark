package graph

import (
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// One direction of one subject lives here: writing a spawn, a join and a
// cancel back as a script. A flow names a thread by the binding the script gave
// its spawn, never by an id: the run assigns ids, and a flow that predicted one
// would be a guess. The other direction is in spawn_read.go.

// Spawn is a spawn, bound when the script named it.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Spawn
//   - 2026-11-02 10:25: moved to spawn_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: comment now names Spawn, were _Spawn (C120a, Q314)
func _Spawn(spawn *workflowpb.Spawn) (syntax.Stmt, error) {
	if spawn == nil {
		return nil, fmt.Errorf(
			"spawn: %w",
			ErrForm,
		)
	}

	target, err := _Target(spawn.GetCall())
	if err != nil {
		return nil, err
	}

	return _Emit(
		spawn.GetBinding(),
		_Bare(
			common.SPAWN,
			[]syntax.Expr{target},
		),
	), nil
}

// _Join is a join, bound when the script captures the value.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Join
//   - 2026-11-02 10:25: moved to spawn_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
func _Join(join *workflowpb.Join) (syntax.Stmt, error) {
	if join == nil {
		return nil, fmt.Errorf(
			"join: %w",
			ErrForm,
		)
	}

	var args []syntax.Expr

	for _, name := range join.GetBindings() {
		args = append(
			args,
			_Name(name),
		)
	}

	return _Emit(
		join.GetResult(),
		_Bare(
			common.JOIN,
			args,
		),
	), nil
}

// Cancel is a cancel.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Cancel
//   - 2026-11-02 10:25: moved to spawn_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: comment now names Cancel, were _Cancel (C120a, Q314)
func _Cancel(cancel *workflowpb.Cancel) (syntax.Stmt, error) {
	if cancel == nil {
		return nil, fmt.Errorf(
			"cancel: %w",
			ErrForm,
		)
	}

	var args []syntax.Expr

	for _, name := range cancel.GetBindings() {
		args = append(
			args,
			_Name(name),
		)
	}

	return &syntax.ExprStmt{
		X: _Bare(
			common.CANCEL,
			args,
		),
	}, nil
}
