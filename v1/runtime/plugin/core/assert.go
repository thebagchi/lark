package core

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrAssert is what a script raises through assert, which is the only way
	// it can signal failure: raise is a reserved lexer keyword and a builtin of
	// that name will not parse as a call.
	ErrAssert = errors.New("assertion failed")

	// ErrNotACondition is returned for assert("text"), which reads like an
	// unconditional failure and behaves like a passing test.
	ErrNotACondition = errors.New("assert wants a condition")
)

// _Assert fails the run when cond is false, and does nothing when it is true.
//
// Three shapes, and the third is why the first two are not enough:
//
//	assert(x == 1)                 fails when x is not 1
//	assert(x == 1, "why")          the same, carrying a message
//	assert(msg = "unreachable")    always fails
//
// The keyword form exists because a bare assert("text") reads like the third
// and behaves like the first: a non-empty string is true, so it passes
// silently. That is refused rather than being quietly correct for one spelling
// of a message and quietly wrong for another.
//
// Starlark's own fail() is the other way a script stops, and the two are not
// interchangeable: fail ends the thread it ran on, this ends the run, unless
// something is catching assertions - retry is what draws that line. A step
// that could be retried should assert; one that must abort whatever else is
// happening should fail.
//
// A refused call fails the run exactly as a failed one does. The author wrote
// an assertion, and whatever they got wrong, they meant "fail here". The
// sentinel still differs, so a host can tell a malformed script from a failing
// one.
//
// Returns ErrAssert when the condition is false or the keyword form was used,
// and ErrNotACondition when the only argument is a string.
//
// Revisions:
//   - 2026-09-19 20:46: initial creation
//   - 2026-09-19 23:58: takes msg as a keyword for an unconditional failure,
//     and refuses a lone string, which used to pass silently
//   - 2026-09-20 00:09: stops the run rather than only the calling thread
//   - 2026-09-20 00:14: a refused call stops the run too, because a mistyped
//     assertion is still an assertion
//   - 2026-09-21 08:09: fails through Fail, the one place that asks whether
//     something is catching
//   - 2026-09-21 09:46: moved here from the scheduler
func _Assert(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		cond starlark.Value
		msg  starlark.Value
	)

	err := starlark.UnpackArgs(ASSERT, args, kwargs, "cond?", &cond, "msg?", &msg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ASSERT, err)
	}

	if cond == nil {
		return nil, scheduler.Fail(thread, _Failure(msg))
	}

	text, bare := cond.(starlark.String)
	if bare && len(args) == 1 {
		return nil, scheduler.Fail(thread, fmt.Errorf(
			"%s got only the message %s: write %s(False, msg) to fail, or %s(msg = ...): %w",
			ASSERT,
			text.String(),
			ASSERT,
			ASSERT,
			ErrNotACondition,
		))
	}

	if cond.Truth() {
		return starlark.None, nil
	}

	return nil, scheduler.Fail(thread, _Failure(msg))
}

// _Failure is the error a failed assertion raises, with the message a script
// gave it or without one.
//
// Revisions:
//   - 2026-09-19 23:59: initial creation
func _Failure(msg starlark.Value) error {
	if msg == nil {
		return ErrAssert
	}

	return fmt.Errorf("%s: %w", msg.String(), ErrAssert)
}
