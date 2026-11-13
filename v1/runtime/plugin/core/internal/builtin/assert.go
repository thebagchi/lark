// Package builtin holds the thread and outcome builtins of the core plugin:
// assert, spawn, join, wait, sleep and the reach walk that finds a value to
// join. The plugin type, its registration, cancel, stop and show stay in package
// core.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package builtin

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrAssert is what a script raises through assert, which is the only way
	// it can signal failure: raise is a reserved lexer keyword and a builtin of
	// that name will not parse as a call.
	//
	// Revisions:
	//   - 2026-09-19 23:36: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
	ErrAssert = errors.New("assertion failed")

	// ErrCondition is returned for assert("text"), which reads like an
	// unconditional failure and behaves like a passing test.
	//
	// Revisions:
	//   - 2026-09-19 23:58: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
	//   - 2026-11-10 17:15: renamed from ErrNotACondition (C121, Q348)
	ErrCondition = errors.New("wants a condition")
)

// Assert fails the run when cond is false, and does nothing when it is true.
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
// and ErrCondition when the only argument is a string.
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
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-04 07:10: the failure carries the position of the call (C69)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Assert to Assert, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrCondition, formerly ErrNotACondition (C121, Q348)
func Assert(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		cond starlark.Value
		msg  starlark.Value
	)

	err := starlark.UnpackArgs(
		spec.ASSERT,
		args,
		kwargs,
		"cond?",
		&cond,
		"msg?",
		&msg,
	)
	if err != nil {
		return nil, err
	}

	if cond == nil {
		return nil, _Refused(
			thread,
			Failure(msg),
		)
	}

	return _Bare(
		thread,
		cond,
		args,
		msg,
	)
}

// _Bare refuses a call that passed only a message, and fails a false condition.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-04 07:10: fails through _Refused, which adds the position (C69)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrCondition, formerly ErrNotACondition (C121, Q348)
func _Bare(
	thread *starlark.Thread,
	cond starlark.Value,
	args starlark.Tuple,
	msg starlark.Value,
) (starlark.Value, error) {
	text, bare := cond.(starlark.String)
	if bare && len(args) == 1 {
		return nil, _Refused(
			thread,
			fmt.Errorf(
				"%s got only the message %s: write %s(False, msg) to fail, "+
					"or %s(msg = ...): %w",
				spec.ASSERT,
				text.String(),
				spec.ASSERT,
				spec.ASSERT,
				ErrCondition,
			),
		)
	}

	if cond.Truth() {
		return starlark.None, nil
	}

	return nil, _Refused(
		thread,
		Failure(msg),
	)
}

// _Refused fails the run with cause, carrying the position of the assert call
// that raised it, and returns cause for the builtin to raise.
//
// Revisions:
//   - 2026-11-04 07:10: initial creation (C69)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func _Refused(
	thread *starlark.Thread,
	cause error,
) error {
	return scheduler.Fail(
		thread,
		scheduler.Locate(
			thread,
			cause,
		),
	)
}

// Failure is the error a failed assertion raises, with the message a script
// gave it or without one.
//
// A message whose String panics is a message that cannot be read, and it gives
// ErrAssert alone, as no message does. The panic text stays out of the error,
// because it is not the script's to read.
//
// Revisions:
//   - 2026-09-19 23:59: initial creation
//   - 2026-11-04 00:30: a message whose String panics gives ErrAssert alone (C54, B095)
//   - 2026-11-05 08:07: the text is "assertion failed: msg", the reason first (C70, Q159),
//     and the message is cut at the Q4 bound
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Failure to Failure, for
//     the tests in tests/ (C120b, Q315, Q314)
func Failure(msg starlark.Value) error {
	if msg == nil {
		return ErrAssert
	}

	var (
		text string
		err  error
	)

	guard.WithRecover(
		&text,
		&err,
		func() (string, error) {
			return msg.String(), nil
		},
	)

	if err != nil {
		return ErrAssert
	}

	return fmt.Errorf(
		"%w: %s",
		ErrAssert,
		unpack.ShortText(text),
	)
}
