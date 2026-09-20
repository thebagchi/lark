// Package flow gives a script the wrappers that repeat, retry and bound a call.
// Importing it is what enables it.
//
// Each is a factory: it takes a function and returns a callable. The wrapping
// happens when that callable is invoked, not when it is built, so a script can
// build one once and call it many times.
package flow

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/plugin"
)

var (
	// ErrNotAName is returned for a target that cannot be named.
	ErrNotAName = errors.New("wants a named function")

	// ErrCount is returned for a count or an attempt limit below one.
	ErrCount = errors.New("must be at least 1")

	// ErrAttempt is returned when n() is called outside a wrapper.
	ErrAttempt = errors.New("n() is only meaningful inside repeat or retry")
)

const (
	NAME        = "flow"
	REPEAT      = "repeat"
	RETRY       = "retry"
	TIMEOUT     = "timeout"
	ATTEMPT     = "n"
	LAMBDA      = "lambda"
	ATTEMPT_KEY = "flow.attempt"

	// UNCOUNTED is the attempt a function reports when nothing is counting its
	// calls, which is the same zero n() gives outside a repeat or a retry.
	UNCOUNTED = 0
	ONCE      = 1
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

// _Named reads a factory's arguments as a named function and a count.
//
// A lambda is refused. These wrappers name the function they wrap, in an error
// and in whatever a graph records, and a lambda has no name to give.
//
// Revisions:
//   - 2026-09-20 01:08: initial creation
func _Named(
	who string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*starlark.Function, int, error) {
	var (
		count  int
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

// _Attempt returns the 1-based attempt number of the repeat or retry this call
// is inside.
//
// Returns ErrAttempt outside one, rather than a number that would be a guess.
//
// Revisions:
//   - 2026-09-20 01:11: initial creation
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

	attempt, ok := thread.Local(ATTEMPT_KEY).(int)
	if !ok {
		return nil, fmt.Errorf("%s: %w", fn.Name(), ErrAttempt)
	}

	return starlark.MakeInt(attempt), nil
}

// _Repeat returns a callable that calls its target exactly count times.
//
// It stops at the first error and returns the last success. The same arguments
// go to every call: a repeat is for doing one thing several times, not for
// walking a list.
//
// Each call runs on a goroutine and an interpreter thread of its own, as every
// wrapper here does, so an attempt is one evaluation whether or not anything is
// bounding it.
//
// Revisions:
//   - 2026-09-20 01:13: initial creation
func _Repeat(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	target, count, err := _Named(fn.Name(), args, kwargs)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s(%d, %s)", REPEAT, count, target.Name())

	var last starlark.Value = starlark.None

	for attempt := ONCE; attempt <= count; attempt++ {
		last, err = _Await(thread, target, attempt, false, nil, nil)
		if err != nil {
			failed := fmt.Errorf("%s attempt %d: %w", name, attempt, err)

			_Finished(thread, target.Name(), failed)

			return nil, failed
		}
	}

	_Finished(thread, target.Name(), nil)

	return last, nil
}
