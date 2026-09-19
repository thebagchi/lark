package scheduler

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// ErrAssert is what a script raises through assert, which is the only way it
// can signal failure: raise is a reserved lexer keyword and a builtin of that
// name will not parse as a call.
var (
	ErrAssert = errors.New("assertion failed")

	// ErrNotACondition is returned for assert("text"), which reads like an
	// unconditional failure and behaves like a passing test.
	ErrNotACondition = errors.New("assert wants a condition")
)

const ASSERT = "assert"

// _Assert ends the calling thread when cond is false, and does nothing when it
// is true.
//
// Three shapes, and the third is why the first two are not enough:
//
//	assert(x == 1)                 fails when x is not 1
//	assert(x == 1, "why")          the same, carrying a message
//	assert(msg = "unreachable")    always fails
//
// The keyword form exists because a bare assert("text") reads like the third
// and behaves like the first: a non-empty string is true, so it passes
// silently. That is refused now rather than being quietly correct for one
// spelling of a message and quietly wrong for another.
//
// A script cannot raise: raise is a reserved lexer keyword, so a builtin of
// that name will not parse as a call. Returning an error from here is how a
// script signals failure, and a Starlark error unwinds the thread it was raised
// on and no other.
//
// A failed assertion stops the whole run, not only the thread it ran on. See
// _Stop.
//
// Returns ErrAssert when the condition is false or the keyword form was used,
// and ErrNotACondition when the only argument is a string.
//
// A refused call does **not** stop the run. It is a malformed call rather than
// an assertion - the script says nothing about whether anything is wrong, only
// that it was written wrongly - so it ends the calling thread like any other
// error and surfaces where something joins it.
//
// Revisions:
//   - 2026-09-19 20:46: initial creation
//   - 2026-09-19 23:58: takes msg as a keyword for an unconditional failure,
//     and refuses a lone string, which used to pass silently
//   - 2026-09-20 00:09: stops the run rather than only the calling thread
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
		return nil, _Stop(thread, _Failure(msg))
	}

	text, bare := cond.(starlark.String)
	if bare && len(args) == 1 {
		return nil, fmt.Errorf(
			"%s got only the message %s: write %s(False, msg) to fail, or %s(msg = ...): %w",
			ASSERT,
			text.String(),
			ASSERT,
			ASSERT,
			ErrNotACondition,
		)
	}

	if cond.Truth() {
		return starlark.None, nil
	}

	return nil, _Stop(thread, _Failure(msg))
}

// _Stop ends the run this thread belongs to, recording cause as its outcome,
// and returns cause unchanged.
//
// A failed assertion stops everything, not only the thread it ran on: the
// spine, every spawned thread, and anything they spawned. That is what a test
// runner does - the first assertion ends the test - and it is why assert is
// not simply an error a script could have returned.
//
// The outcome is recorded before anything is cancelled, so the run's answer is
// already fixed by the time the shutdown starts producing errors of its own.
// This thread's handle will report cancellation like any other, and that is
// correct: what happened to the thread and why the run ended are two
// questions.
//
// Revisions:
//   - 2026-09-20 00:09: initial creation
func _Stop(thread *starlark.Thread, cause error) error {
	run, err := _Of(thread)
	if err != nil {
		return cause
	}

	run._End(cause)

	return cause
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
