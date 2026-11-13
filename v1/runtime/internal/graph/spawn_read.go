package graph

import (
	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// Cancel is the statement a cancel is, and whether it models.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to spawn_read.go
//   - 2026-11-08 10:35: a nil call is no cancel (C108, B189)
//   - 2026-11-10 00:25: renamed from _Cancel, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Cancel(
	call *syntax.CallExpr,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	if call == nil {
		return nil, false
	}

	bindings, ok := _Bindings(
		call.Args,
		scope,
	)
	if !ok {
		return nil, false
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Cancel{
			Cancel: &workflowpb.Cancel{
				Bindings: bindings,
			},
		},
	}, true
}

// One direction of one subject lives here: reading a spawn, a join and a
// cancel out of a script. A flow names a thread by the binding the script gave
// its spawn, never by an id: the run assigns ids, and a flow that predicted one
// would be a guess. The other direction is in spawn_write.go.

// Spawn is the statement a spawn is, bound to bound when the script named it,
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
//   - 2026-11-02 10:25: moved to spawn_read.go
//   - 2026-11-10 01:00: now uses Spawn, Reading, Scope, Target, and 1 more, were _Spawn, _Reading,
//     _Scope, _Target, and 1 more (C120a, Q314)
func (r *Reading) _Spawn(
	call *syntax.CallExpr,
	bound string,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	if len(call.Args) != 1 {
		return nil, false
	}

	target, ok := r.Target(
		call.Args[0],
		scope,
	)
	if !ok {
		return nil, false
	}

	if bound != "" && !scope.Fork(bound) {
		return nil, false
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Spawn{
			Spawn: &workflowpb.Spawn{
				Binding: bound,
				Call:    target,
			},
		},
	}, true
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
//   - 2026-11-02 10:25: moved to spawn_read.go
//   - 2026-11-10 01:00: now uses Reading, Scope, Bind, were _Reading, _Scope, _Bind (C120a, Q314)
func (r *Reading) _Join(
	call *syntax.CallExpr,
	bound string,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	bindings, ok := _Bindings(
		call.Args,
		scope,
	)
	if !ok {
		return nil, false
	}

	if bound != "" {
		scope.Bind(bound)
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Join{
			Join: &workflowpb.Join{
				Bindings: bindings,
				Result:   bound,
			},
		},
	}, true
}

// _Bindings is the spawn bindings a join or a cancel names, and whether every
// argument is one.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to spawn_read.go
//   - 2026-11-10 01:00: now uses Scope, Spawned, were _Scope, _Spawned (C120a, Q314)
func _Bindings(
	args []syntax.Expr,
	scope *Scope,
) ([]string, bool) {
	var bindings []string

	for _, arg := range args {
		name, ok := arg.(*syntax.Ident)
		if !ok || !scope.Spawned(name.Name) {
			return nil, false
		}

		bindings = append(
			bindings,
			name.Name,
		)
	}

	return bindings, true
}
