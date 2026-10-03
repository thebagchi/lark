package core

import (
	"errors"
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ERR_INTERRUPTED is returned when a sleep is cut short by a cancelled run.
var ERR_INTERRUPTED = errors.New("sleep was interrupted")

// _Sleep pauses this evaluation for the given number of milliseconds.
//
// It waits on the evaluation's context as well as the clock, and that is the
// whole point of it. Cancelling a run cancels the interpreter thread, which
// takes effect between instructions - it does nothing at all to a Go call
// already blocked. A sleep written as a plain time.Sleep would therefore
// ignore a cancel, and a run would hang for the full duration rather than
// ending. Everything blocking in this runtime owes the same debt.
//
// The argument is milliseconds, the unit the schema stores. A sleep is a line
// of the thread that sleeps, reported around the wait.
//
// Returns ERR_INTERRUPTED when the evaluation ends first, so a script can tell
// a sleep that finished from one that was cut short.
//
// Revisions:
//   - 2026-09-20 01:02: initial creation
//   - 2026-09-21 08:09: reads its argument through Duration
//   - 2026-09-21 09:46: moved here from the scheduler
//   - 2026-10-02 00:48: reports itself as a line, and takes milliseconds
func _Sleep(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	scheduler.Open(thread, &scheduler.Line{Builtin: SLEEP})

	value, err := _Slept(thread, fn, args, kwargs)

	scheduler.Close(thread, "", err)

	return value, err
}

// _Slept reads the duration and waits it out, or until the evaluation ends.
//
// Revisions:
//   - 2026-10-02 00:48: initial creation, from _Sleep's body, so the line
//     _Sleep reports closes whichever way this ends
func _Slept(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var given starlark.Value

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	pause, err := scheduler.Duration(given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	ctx, err := scheduler.Context(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	timer := time.NewTimer(pause)
	defer timer.Stop()

	select {
	case <-timer.C:
		return starlark.None, nil

	case <-ctx.Done():
		return nil, fmt.Errorf("%s: %w: %w", fn.Name(), ERR_INTERRUPTED, ctx.Err())
	}
}
