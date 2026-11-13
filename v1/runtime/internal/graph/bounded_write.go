package graph

import (
	"fmt"
	"strconv"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// One direction of one subject lives here: writing a sleep and the wrappers
// that repeat, retry and bound a call back as a script. A script and the schema
// both count milliseconds, so nothing converts. The other direction is in
// bounded_read.go.

// Sleep is a sleep, in the milliseconds the builtin and the schema count.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, writing milliseconds, which the
//     builtin now takes
//   - 2026-11-02 10:25: moved to bounded_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: comment now names Sleep, were _Sleep (C120a, Q314)
func _Sleep(sleep *workflowpb.Sleep) (syntax.Stmt, error) {
	if sleep == nil || sleep.GetDurationMs() < 0 {
		return nil, fmt.Errorf(
			"sleep: %w",
			ErrForm,
		)
	}

	pause := _Num(strconv.Itoa(int(sleep.GetDurationMs())))

	return &syntax.ExprStmt{
		X: _Bare(
			common.SLEEP,
			[]syntax.Expr{pause},
		),
	}, nil
}

// _Repeat is a repeat. A delay is written only when there is one.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Repeat
//   - 2026-11-02 10:25: moved to bounded_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
func _Repeat(rep *workflowpb.Repeat) (syntax.Stmt, error) {
	return _Attempts(
		common.REPEAT,
		rep.GetCall(),
		rep.GetCount(),
		rep.GetDelayMs(),
	)
}

// _Retry is a retry. A delay is written only when there is one.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Retry
//   - 2026-11-02 10:25: moved to bounded_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
func _Retry(ret *workflowpb.Retry) (syntax.Stmt, error) {
	return _Attempts(
		common.RETRY,
		ret.GetCall(),
		ret.GetAttempts(),
		ret.GetDelayMs(),
	)
}

// _Attempts is repeat or retry: a count, a target, and a delay when one is set.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, writing the delay in milliseconds,
//     which the builtin now takes
//   - 2026-11-02 10:25: moved to bounded_write.go
func _Attempts(
	word string,
	call *workflowpb.Call,
	count,
	delay int32,
) (syntax.Stmt, error) {
	if count < 0 || delay < 0 {
		return nil, fmt.Errorf(
			"%s: %w",
			word,
			ErrForm,
		)
	}

	target, err := _Target(call)
	if err != nil {
		return nil, err
	}

	args := []syntax.Expr{_Num(strconv.Itoa(int(count))), target}

	if delay != 0 {
		args = append(
			args,
			_Num(strconv.Itoa(int(delay))),
		)
	}

	return &syntax.ExprStmt{
		X: _Bare(
			word,
			args,
		),
	}, nil
}

// Timeout is a timeout, in the milliseconds the builtin and the schema count.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, writing milliseconds, which the
//     builtin now takes
//   - 2026-11-02 10:25: moved to bounded_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: comment now names Timeout, were _Timeout (C120a, Q314)
func _Timeout(held *workflowpb.Timeout) (syntax.Stmt, error) {
	if held == nil || held.GetTimeoutMs() < 0 {
		return nil, fmt.Errorf(
			"timeout: %w",
			ErrForm,
		)
	}

	target, err := _Target(held.GetCall())
	if err != nil {
		return nil, err
	}

	args := []syntax.Expr{_Num(strconv.Itoa(int(held.GetTimeoutMs()))), target}

	return &syntax.ExprStmt{
		X: _Bare(
			common.TIMEOUT,
			args,
		),
	}, nil
}
