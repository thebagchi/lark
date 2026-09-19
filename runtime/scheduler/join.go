package scheduler

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// ErrNotAHandle is returned when join or cancel is given something that is not
// a handle, or any keyword argument.
var ErrNotAHandle = errors.New("join wants handles")

const (
	JOIN   = "join"
	CANCEL = "cancel"
)

// _Join waits for every handle given and returns what each produced, in the
// order they were passed.
//
// A thread that failed re-raises here, in the thread that joined it, so a
// caller cannot take a result list and be unaware that one of them never
// arrived. The first failure in argument order wins, which makes the outcome
// depend on how the script was written rather than on which thread lost a race.
//
// Returns ErrNotAHandle when an argument is something else, and ErrCancelled
// when a joined handle was cancelled.
//
// Revisions:
//   - 2026-09-19 20:42: initial creation
func _Join(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	handles, err := _Handles(JOIN, args, kwargs)
	if err != nil {
		return nil, err
	}

	for _, handle := range handles {
		<-handle.done
	}

	values := make([]starlark.Value, 0, len(handles))

	for _, handle := range handles {
		if handle.err != nil {
			return nil, fmt.Errorf("%s: %w", handle.name, handle.err)
		}

		values = append(values, handle.value)
	}

	return starlark.NewList(values), nil
}

// _Cancel stops every handle given, without waiting for any of them.
//
// Cancelling is not an outcome: a joined handle that was cancelled raises,
// because a caller asking for its result is asking for something that will
// never exist.
//
// Revisions:
//   - 2026-09-19 20:44: initial creation
func _Cancel(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	handles, err := _Handles(CANCEL, args, kwargs)
	if err != nil {
		return nil, err
	}

	for _, handle := range handles {
		handle.stop()
	}

	return starlark.None, nil
}

// _Handles reads a builtin's arguments as a list of handles.
//
// Revisions:
//   - 2026-09-19 20:45: initial creation
func _Handles(name string, args starlark.Tuple, kwargs []starlark.Tuple) ([]*Handle, error) {
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("%s takes no keyword arguments: %w", name, ErrNotAHandle)
	}

	handles := make([]*Handle, 0, len(args))

	for _, arg := range args {
		handle, ok := arg.(*Handle)
		if !ok {
			return nil, fmt.Errorf("%s got %s: %w", name, arg.Type(), ErrNotAHandle)
		}

		handles = append(handles, handle)
	}

	return handles, nil
}
