// Package flow gives a script the wrappers that repeat, retry and bound a call.
// Importing it is what enables it.
//
// Each wrapper takes a count or a budget first and the function last, and
// calls it straight away: what it gives back is what the wrapped call
// produced. Every attempt runs beside the caller on a thread of its own, on
// the caller's own lane, through the scheduler's one primitive for that.
package flow

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/scheduler"
)

var (
	// ErrCount is returned for a count or an attempt limit below one.
	ErrCount = errors.New("must be at least 1")

	// ErrAttempt is returned when n() is called outside a wrapper.
	ErrAttempt = errors.New("n() is only meaningful inside repeat or retry")
)

const (
	NAME    = "flow"
	REPEAT  = "repeat"
	RETRY   = "retry"
	TIMEOUT = "timeout"
	ATTEMPT = "n"
	ONCE    = 1
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

// _Counted reads a wrapper's arguments as a count and the function to call.
//
// A lambda is accepted: a compiler emits one wherever a call passes
// arguments, since a wrapper takes none to pass on. What that costs is the
// name, which the graph supplies where it can.
//
// Revisions:
//   - 2026-09-20 01:08: initial creation, as _Named
//   - 2026-09-21 08:09: named for what it reads, since it refuses no lambda
func _Counted(
	who string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*starlark.Function, int32, error) {
	var (
		count  int32
		target *starlark.Function
	)

	err := starlark.UnpackPositionalArgs(who, args, kwargs, 2, &count, &target)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", who, err)
	}

	if count < ONCE {
		return nil, 0, fmt.Errorf("%s got %d: %w", who, count, ErrCount)
	}

	return target, count, nil
}

// _Attempt returns the 1-based attempt number of the repeat or retry this
// evaluation, or the evaluation that started it, is inside.
//
// Returns ErrAttempt outside one, rather than a number that would be a guess.
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
		return nil, fmt.Errorf("%s: %w", fn.Name(), ErrAttempt)
	}

	return starlark.MakeInt(int(attempt)), nil
}

// _Repeat calls its target exactly count times.
//
// It stops at the first error and returns the last success. The same arguments
// go to every call: a repeat is for doing one thing several times, not for
// walking a list.
//
// Revisions:
//   - 2026-09-20 01:13: initial creation
//   - 2026-09-21 08:09: each attempt is Beside on the caller's lane
func _Repeat(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	target, count, err := _Counted(fn.Name(), args, kwargs)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s(%d, %s)", REPEAT, count, target.Name())

	var last starlark.Value = starlark.None

	for attempt := int32(ONCE); attempt <= count; attempt++ {
		last, err = _Attempted(thread, target, attempt)
		if err != nil {
			failed := fmt.Errorf("%s attempt %d: %w", name, attempt, err)

			_Finished(thread, target.Name(), failed)

			return nil, failed
		}
	}

	_Finished(thread, target.Name(), nil)

	return last, nil
}

// _Attempted runs attempt n of target beside the caller and waits for it,
// telling whatever is watching that the attempt began.
//
// Revisions:
//   - 2026-09-20 01:28: initial creation, as _Await
//   - 2026-09-21 08:09: Beside and Wait, counted as an attempt, plus whatever
//     else a caller asks for
func _Attempted(
	thread *starlark.Thread,
	target *starlark.Function,
	attempt int32,
	opts ...scheduler.Option,
) (starlark.Value, error) {
	counted := append([]scheduler.Option{scheduler.Attempt(attempt)}, opts...)

	handle, err := scheduler.Beside(thread, target, counted...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", target.Name(), err)
	}

	_Began(thread, target.Name(), attempt)

	return scheduler.Wait(thread, handle)
}
