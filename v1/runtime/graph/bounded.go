package graph

import (
	"fmt"
	"strconv"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// Both directions of one subject live here: reading a sleep and the wrappers
// that repeat, retry and bound a call out of a script, and writing them back.
// A script and the schema both count milliseconds, so nothing converts.

const (
	// WRAPPED is how many arguments a wrapper takes before its delay: how
	// much, and what to do that much of. DELAYED is the count with a delay
	// passed by position.
	WRAPPED = 2
	DELAYED = 3

	// ONCE is the fewest calls a repeat makes and the fewest attempts a retry
	// may make. The builtin refuses fewer, so fewer is no statement.
	ONCE = 1
)

// _Sleep is the statement a sleep is, and whether it models.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation, as _Pauses
//   - 2026-09-21 08:09: a number only; sleep("1") used to derive as 0
//   - 2026-10-02 00:04: reads milliseconds, which the builtin now takes, and
//     returns the Statement
func (r *_Reading) _Sleep(call *syntax.CallExpr) (*workflowpb.Statement, bool) {
	if len(call.Args) != 1 {
		return nil, false
	}

	pause, ok := _Natural(call.Args[0])
	if !ok {
		return nil, false
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Sleep{
		Sleep: &workflowpb.Sleep{DurationMs: pause},
	}}, true
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
func (r *_Reading) _Attempts(
	name string,
	call *syntax.CallExpr,
	scope *_Scope,
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

	target, ok := r._Target(positional[1], scope)
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

	if name == REPEAT {
		return &workflowpb.Statement{Action: &workflowpb.Statement_Repeat{
			Repeat: &workflowpb.Repeat{Call: target, Count: count, DelayMs: pause},
		}}, true
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Retry{
		Retry: &workflowpb.Retry{Call: target, Attempts: count, DelayMs: pause},
	}}, true
}

// _Timeout is the statement a timeout is, and whether it models.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Timeout(call *syntax.CallExpr, scope *_Scope) (*workflowpb.Statement, bool) {
	if len(call.Args) != WRAPPED {
		return nil, false
	}

	budget, ok := _Natural(call.Args[0])
	if !ok {
		return nil, false
	}

	target, ok := r._Target(call.Args[1], scope)
	if !ok {
		return nil, false
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Timeout{
		Timeout: &workflowpb.Timeout{Call: target, TimeoutMs: budget},
	}}, true
}

// _Delayed is a wrapper's arguments split in two: those passed by position,
// and the value of the delay keyword when one is named.
//
// Any other keyword, or an unpacked *args or **kwargs, is a call the schema has
// no place for.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
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
			positional = append(positional, arg)

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
func _Count(expr syntax.Expr) (int32, bool) {
	count, ok := _Natural(expr)

	counted := ok && count >= ONCE
	if !counted {
		return 0, false
	}

	return count, true
}

// _Sleep is a sleep, in the milliseconds the builtin and the schema count.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, writing milliseconds, which the
//     builtin now takes
func _Sleep(sleep *workflowpb.Sleep) (syntax.Stmt, error) {
	if sleep == nil || sleep.GetDurationMs() < 0 {
		return nil, fmt.Errorf("sleep: %w", ERR_FORM)
	}

	pause := _Num(strconv.Itoa(int(sleep.GetDurationMs())))

	return &syntax.ExprStmt{X: _Bare(SLEEP, []syntax.Expr{pause})}, nil
}

// _Repeat is a repeat. A delay is written only when there is one.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Repeat
func _Repeat(rep *workflowpb.Repeat) (syntax.Stmt, error) {
	return _Attempts(REPEAT, rep.GetCall(), rep.GetCount(), rep.GetDelayMs())
}

// _Retry is a retry. A delay is written only when there is one.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Retry
func _Retry(ret *workflowpb.Retry) (syntax.Stmt, error) {
	return _Attempts(RETRY, ret.GetCall(), ret.GetAttempts(), ret.GetDelayMs())
}

// _Attempts is repeat or retry: a count, a target, and a delay when one is set.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, writing the delay in milliseconds,
//     which the builtin now takes
func _Attempts(word string, call *workflowpb.Call, count, delay int32) (syntax.Stmt, error) {
	if count < 0 || delay < 0 {
		return nil, fmt.Errorf("%s: %w", word, ERR_FORM)
	}

	target, err := _Target(call)
	if err != nil {
		return nil, err
	}

	args := []syntax.Expr{_Num(strconv.Itoa(int(count))), target}

	if delay != 0 {
		args = append(args, _Num(strconv.Itoa(int(delay))))
	}

	return &syntax.ExprStmt{X: _Bare(word, args)}, nil
}

// _Timeout is a timeout, in the milliseconds the builtin and the schema count.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, writing milliseconds, which the
//     builtin now takes
func _Timeout(held *workflowpb.Timeout) (syntax.Stmt, error) {
	if held == nil || held.GetTimeoutMs() < 0 {
		return nil, fmt.Errorf("timeout: %w", ERR_FORM)
	}

	target, err := _Target(held.GetCall())
	if err != nil {
		return nil, err
	}

	args := []syntax.Expr{_Num(strconv.Itoa(int(held.GetTimeoutMs()))), target}

	return &syntax.ExprStmt{X: _Bare(TIMEOUT, args)}, nil
}
