package scheduler

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// ErrAssert is what a script raises through assert, which is the only way it
// can signal failure: raise is a reserved lexer keyword and a builtin of that
// name will not parse as a call.
var ErrAssert = errors.New("scheduler: assertion failed")

const ASSERT = "assert"

// _Assert ends the calling thread when cond is false, and does nothing when it
// is true.
//
// A script cannot raise: raise is a reserved lexer keyword, so a builtin of
// that name will not parse as a call. Returning an error from here is how a
// script signals failure, and a Starlark error unwinds the thread it was
// raised on and no other.
//
// Revisions:
//   - 2026-09-19 20:46: initial creation
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

	err := starlark.UnpackPositionalArgs(ASSERT, args, kwargs, 1, &cond, &msg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ASSERT, err)
	}

	if cond.Truth() {
		return starlark.None, nil
	}

	if msg == nil {
		return nil, ErrAssert
	}

	return nil, fmt.Errorf("%s: %w", msg.String(), ErrAssert)
}
