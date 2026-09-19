package scheduler

import (
	"errors"
	"fmt"
	"time"

	"go.starlark.net/starlark"
)

// ErrInterrupted is returned when a sleep is cut short by a cancelled run.
var ErrInterrupted = errors.New("sleep was interrupted")

const (
	SLEEP = "sleep"

	// NANOS is how many of time.Duration's units make a second, so a script's
	// fractional seconds can become one.
	NANOS = float64(time.Second)
)

// _Sleep pauses this evaluation for the given number of seconds.
//
// It waits on the run's context as well as the clock, and that is the whole
// point of it. Cancelling a run cancels the interpreter thread, which takes
// effect between instructions - it does nothing at all to a Go call already
// blocked. A sleep written as a plain time.Sleep would therefore ignore a
// cancel, and a run would hang for the full duration rather than ending, with
// the thread that is waiting for it none the wiser.
//
// This is the first builtin here that blocks, so it is the first place that
// rule has teeth. Everything blocking that follows owes the same debt.
//
// Returns ErrInterrupted when the run ends first, so a script can tell a sleep
// that finished from one that was cut short.
//
// Revisions:
//   - 2026-09-20 01:02: initial creation
func _Sleep(
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

	// Taken as a number rather than as a float, so sleep(1) works. Demanding a
	// float would refuse the most obvious way to write a whole second.
	seconds, ok := starlark.AsFloat(given)
	if !ok {
		return nil, fmt.Errorf("%s got %s: %w", fn.Name(), given.Type(), ErrDuration)
	}

	if seconds < 0 {
		return nil, fmt.Errorf("%s: %g is negative: %w", fn.Name(), seconds, ErrDuration)
	}

	ctx, err := Context(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	timer := time.NewTimer(time.Duration(seconds * NANOS))
	defer timer.Stop()

	select {
	case <-timer.C:
		return starlark.None, nil

	case <-ctx.Done():
		return nil, fmt.Errorf("%s: %w: %w", fn.Name(), ErrInterrupted, ctx.Err())
	}
}
