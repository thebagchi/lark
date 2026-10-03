// Package flow gives a script the wrappers that repeat, retry and bound a call.
// Importing it is what enables it.
//
// Each wrapper takes a count or a budget first and the function second, and
// calls it straight away: what it gives back is what the wrapped call
// produced. A repeat or a retry may take a delay third, the milliseconds it
// waits between calls, and a timeout's budget is milliseconds too. Every
// attempt runs beside the caller on a thread of its own, on the caller's own
// lane, through the scheduler's one primitive for that.
package flow

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

var (
	// ERR_COUNT is returned for a count or an attempt limit below one.
	ERR_COUNT = errors.New("must be at least 1")

	// ERR_ATTEMPT is returned when n() is called outside a wrapper.
	ERR_ATTEMPT = errors.New("n() is only meaningful inside repeat or retry")
)

const (
	NAME    = "flow"
	REPEAT  = spelling.REPEAT
	RETRY   = spelling.RETRY
	TIMEOUT = spelling.TIMEOUT
	ATTEMPT = "n"

	// DELAY is the keyword a repeat or a retry takes its delay by, when the
	// delay is not passed third.
	DELAY = "delay"

	// ONCE is the fewest calls a repeat makes and the fewest attempts a retry
	// may make. WRAPPED is how many arguments every wrapper takes before a
	// delay: how much, and what to do that much of.
	ONCE    = 1
	WRAPPED = 2
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation
func init() {
	plugin.Register(&_Flow{})
}

// _Flow is the plugin. Empty: a wrapper holds what it needs, so there is
// nothing here.
type _Flow struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation
func (f *_Flow) Name() string {
	return NAME
}

// Values returns the four names this plugin supplies.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation
func (f *_Flow) Values() starlark.StringDict {
	return starlark.StringDict{
		REPEAT:  starlark.NewBuiltin(REPEAT, _Repeat),
		RETRY:   starlark.NewBuiltin(RETRY, _Retry),
		TIMEOUT: starlark.NewBuiltin(TIMEOUT, _Timeout),
		ATTEMPT: starlark.NewBuiltin(ATTEMPT, _Attempt),
	}
}

// _Attempt returns the 1-based attempt number of the repeat or retry this
// evaluation, or the evaluation that started it, is inside.
//
// Returns ERR_ATTEMPT outside one, rather than a number that would be a guess.
//
// Revisions:
//   - 2026-09-20 01:11: initial creation
//   - 2026-09-21 08:09: reads the attempt the scheduler carries, which a
//     spawned thread inherits
func _Attempt(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	attempt := scheduler.AttemptOf(thread)
	if attempt == scheduler.NO_ATTEMPT {
		return nil, fmt.Errorf("%s: %w", fn.Name(), ERR_ATTEMPT)
	}

	return starlark.MakeInt(int(attempt)), nil
}

// _Repeat calls its target exactly count times, waiting the delay between
// calls when one is given.
//
// It stops at the first error and returns the last success. The same arguments
// go to every call: a repeat is for doing one thing several times, not for
// walking a list.
//
// Revisions:
//   - 2026-09-20 01:13: initial creation
//   - 2026-09-21 08:09: each attempt is Beside on the caller's lane
//   - 2026-10-02 01:01: takes an optional delay, and reports its line through
//     the scheduler with the count it makes
func _Repeat(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	attempts, err := _Counted(fn.Name(), args, kwargs)
	if err != nil {
		return nil, err
	}

	var last starlark.Value = starlark.None

	for attempt := int32(ONCE); attempt <= attempts.count; attempt++ {
		last, err = attempts._Attempted(thread, attempt)
		if err != nil {
			failed := fmt.Errorf("%s attempt %d: %w", attempts.written, attempt, err)

			scheduler.Close(thread, attempts.name, failed)

			return nil, failed
		}
	}

	scheduler.Close(thread, attempts.name, nil)

	return last, nil
}
