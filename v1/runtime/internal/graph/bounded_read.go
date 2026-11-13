package graph

import (
	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// One direction of one subject lives here: reading a sleep and the wrappers
// that repeat, retry and bound a call out of a script. A script and the schema
// both count milliseconds, so nothing converts. The other direction is in
// bounded_write.go.

// WRAPPED is how many arguments a wrapper takes before its delay: how
// much, and what to do that much of. DELAYED is the count with a delay
// passed by position.
//
// Revisions:
//   - 2026-09-26 23:55: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	WRAPPED = 2
	DELAYED = 3

	// ONCE is the fewest calls a repeat makes and the fewest attempts a retry
	// may make. The builtin refuses fewer, so fewer is no statement.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ONCE = 1
)

// Sleep is the statement a sleep is, and whether it models.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation, as _Pauses
//   - 2026-09-21 08:09: a number only; sleep("1") used to derive as 0
//   - 2026-10-02 00:04: reads milliseconds, which the builtin now takes, and
//     returns the Statement
//   - 2026-11-02 10:25: moved to bounded_read.go
//   - 2026-11-08 10:35: a nil call is no sleep (C108, B186)
//   - 2026-11-10 00:25: renamed from _Sleep, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Sleep(call *syntax.CallExpr) (*workflowpb.Statement, bool) {
	if call == nil || len(call.Args) != 1 {
		return nil, false
	}

	pause, ok := _Natural(call.Args[0])
	if !ok {
		return nil, false
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Sleep{
			Sleep: &workflowpb.Sleep{
				DurationMs: pause,
			},
		},
	}, true
}

// Timeout is the statement a timeout is, and whether it models.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to bounded_read.go
//   - 2026-11-06 22:02: a nil call does not model (C100c, B151)
//   - 2026-11-10 00:25: renamed from _Timeout, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Timeout(
	call *syntax.CallExpr,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	if call == nil || len(call.Args) != WRAPPED {
		return nil, false
	}

	budget, ok := _Natural(call.Args[0])
	if !ok {
		return nil, false
	}

	target, ok := r.Target(
		call.Args[1],
		scope,
	)
	if !ok {
		return nil, false
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Timeout{
			Timeout: &workflowpb.Timeout{
				Call:      target,
				TimeoutMs: budget,
			},
		},
	}, true
}

// _Attempts is the statement a repeat or a retry is, and whether it models.
//
// The count comes first and the function after it, with an optional delay
// third or named delay. The count is a whole number of at least one, and the
// delay a whole number of milliseconds; anything else is a call the builtin
// refuses, and no statement.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation, as _Wrapper
//   - 2026-09-21 08:09: a number only; repeat(True, f) used to derive as 0
//   - 2026-09-30 00:41: reads on a lane, and states no step for a lambda
//     whose arguments it could not carry
//   - 2026-10-02 00:04: reads the optional delay, and leaves the timeout to
//     _Timeout
//   - 2026-11-02 10:25: moved to bounded_read.go
//   - 2026-11-10 01:00: now uses Reading, Scope, Target, were _Reading, _Scope, _Target
//     (C120a, Q314)
func (r *Reading) _Attempts(
	name string,
	call *syntax.CallExpr,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	positional, delay, ok := _Delayed(call.Args)
	if !ok || len(positional) < WRAPPED || len(positional) > DELAYED {
		return nil, false
	}

	if len(positional) == DELAYED {
		if delay != nil {
			return nil, false
		}

		delay = positional[DELAYED-1]
	}

	count, ok := _Count(positional[0])
	if !ok {
		return nil, false
	}

	target, ok := r.Target(
		positional[1],
		scope,
	)
	if !ok {
		return nil, false
	}

	var pause int32

	if delay != nil {
		pause, ok = _Natural(delay)
		if !ok {
			return nil, false
		}
	}

	return _AttemptStmt(
		name,
		target,
		count,
		pause,
	)
}

// _AttemptStmt is the repeat or the retry the call modelled.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:25: moved to bounded_read.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
func _AttemptStmt(
	name string,
	target *workflowpb.Call,
	count int32,
	pause int32,
) (*workflowpb.Statement, bool) {
	if name == common.REPEAT {
		return &workflowpb.Statement{
			Action: &workflowpb.Statement_Repeat{
				Repeat: &workflowpb.Repeat{
					Call:    target,
					Count:   count,
					DelayMs: pause,
				},
			},
		}, true
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Retry{
			Retry: &workflowpb.Retry{
				Call:     target,
				Attempts: count,
				DelayMs:  pause,
			},
		},
	}, true
}

// _Delayed is a wrapper's arguments split in two: those passed by position,
// and the value of the delay keyword when one is named.
//
// Any other keyword, or an unpacked *args or **kwargs, is a call the schema has
// no place for.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to bounded_read.go
func _Delayed(args []syntax.Expr) ([]syntax.Expr, syntax.Expr, bool) {
	var (
		positional []syntax.Expr
		delay      syntax.Expr
	)

	for _, arg := range args {
		if _, unpacked := arg.(*syntax.UnaryExpr); unpacked {
			return nil, nil, false
		}

		keyword, ok := arg.(*syntax.BinaryExpr)
		if !ok || keyword.Op != syntax.EQ {
			positional = append(
				positional,
				arg,
			)

			continue
		}

		name, ok := keyword.X.(*syntax.Ident)
		if !ok || name.Name != DELAY || delay != nil {
			return nil, nil, false
		}

		delay = keyword.Y
	}

	return positional, delay, true
}

// _Count is the count an expression states for a repeat or a retry: a whole
// number of at least one that fits an int32.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-10-03 20:50: the number _Natural reads, held to at least one, rather than
//     a second copy of the reading with another lower bound
//   - 2026-11-02 10:25: moved to bounded_read.go
func _Count(expr syntax.Expr) (int32, bool) {
	var (
		count   int32
		ok      bool
		counted bool
	)

	count, ok = _Natural(expr)

	counted = ok && count >= ONCE
	if !counted {
		return 0, false
	}

	return count, true
}
