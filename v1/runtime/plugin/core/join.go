package core

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ERR_NOT_A_HANDLE is returned when join or cancel is given something that is not
// a handle, or any keyword argument.
var ERR_NOT_A_HANDLE = errors.New("join wants handles")

// _Join waits for every handle given and returns what each produced, in the
// order they were passed.
//
// A thread that failed re-raises here, in the thread that joined it, so a
// caller cannot take a result list and be unaware that one of them never
// arrived. The first failure in argument order wins, which makes the outcome
// depend on how the script was written rather than on which thread lost a race.
//
// A join is a line of the thread that waits, reported around the wait with the
// threads it waits on, so a watcher can say which threads were joined. A join
// refused before it waits is still a line, and it fails.
//
// Returns ERR_NOT_A_HANDLE when an argument is something else, and
// scheduler.ERR_CANCELLED when a joined handle was cancelled or the joining
// evaluation was.
//
// Revisions:
//   - 2026-09-19 20:42: initial creation
//   - 2026-09-20 00:10: cancels the handles it has not reached when one fails,
//     and waits for them, rather than waiting for every handle first
//   - 2026-09-21 08:09: waits through Wait, so the joining evaluation's own
//     cancellation ends the join
//   - 2026-09-21 09:46: moved here from the scheduler
//   - 2026-10-02 00:48: reports itself as a line, naming the threads it waits
//     on
func _Join(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	handles, err := _Handles(JOIN, args, kwargs)

	scheduler.Open(thread, &scheduler.Line{Builtin: JOIN, Waits: _Threads(handles)})

	values, err := _Joined(thread, fn, handles, err)

	scheduler.Close(thread, "", err)

	return values, err
}

// _Joined waits for every handle given, once _Handles has read them, and
// returns what each produced, in the order they were passed.
//
// Revisions:
//   - 2026-10-02 00:48: initial creation, from _Join's body, so the line _Join
//     reports closes whichever way this ends
func _Joined(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	handles []*scheduler.Handle,
	err error,
) (starlark.Value, error) {
	if err != nil {
		return nil, err
	}

	// Refused while a name is held, because the wait can be one nothing in
	// the script can end: an update takes the name before it calls its
	// function, so joining a thread that needs that name leaves the parent
	// waiting for the child and the child waiting for the parent. Only a
	// cancel breaks it, and a script cannot cancel itself.
	//
	// Refused whether or not this evaluation is the one holding the name: a
	// thread that inherited the mark can join into the same cycle.
	updating, err := scheduler.Updating(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	if updating != "" {
		return nil, fmt.Errorf("%s inside an update of %q: %w",
			fn.Name(), updating, scheduler.ERR_NESTED)
	}

	values := make([]starlark.Value, 0, len(handles))

	for index, handle := range handles {
		value, err := scheduler.Wait(thread, handle)
		if err != nil {
			_Abort(thread, handles[index+1:])

			return nil, fmt.Errorf("%s: %w", handle.Name(), err)
		}

		values = append(values, value)
	}

	return starlark.NewList(values), nil
}

// _Cancel stops every handle given, without waiting for any of them.
//
// Cancelling is not an outcome: a joined handle that was cancelled raises,
// because a caller asking for its result is asking for something that will
// never exist. A cancel is a line of the thread that wrote it.
//
// Revisions:
//   - 2026-09-19 20:44: initial creation
//   - 2026-09-21 09:46: moved here from the scheduler
//   - 2026-10-02 00:48: reports itself as a line
func _Cancel(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	scheduler.Open(thread, &scheduler.Line{Builtin: CANCEL})

	handles, err := _Handles(CANCEL, args, kwargs)
	if err != nil {
		scheduler.Close(thread, "", err)

		return nil, err
	}

	for _, handle := range handles {
		handle.Stop()
	}

	scheduler.Close(thread, "", nil)

	return starlark.None, nil
}

// _Threads is the thread each handle runs on, in order.
//
// Revisions:
//   - 2026-10-02 00:48: initial creation
func _Threads(handles []*scheduler.Handle) []string {
	threads := make([]string, 0, len(handles))

	for _, handle := range handles {
		threads = append(threads, handle.Thread())
	}

	return threads
}

// _Handles reads a builtin's arguments as a list of handles.
//
// Revisions:
//   - 2026-09-19 20:45: initial creation
func _Handles(
	name string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]*scheduler.Handle, error) {
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("%s takes no keyword arguments: %w", name, ERR_NOT_A_HANDLE)
	}

	handles := make([]*scheduler.Handle, 0, len(args))

	for _, arg := range args {
		handle, ok := arg.(*scheduler.Handle)
		if !ok {
			return nil, fmt.Errorf("%s got %s: %w", name, arg.Type(), ERR_NOT_A_HANDLE)
		}

		handles = append(handles, handle)
	}

	return handles, nil
}

// _Abort cancels every handle given and waits for each to stop, or for the
// caller's own evaluation to be cancelled.
//
// Waiting is the half that is easy to leave out. Cancelling alone would let a
// join return while the evaluations it gave up on were still unwinding, which
// is how a run ends with goroutines still touching its state.
//
// Revisions:
//   - 2026-09-20 00:11: initial creation
//   - 2026-09-21 08:09: watches the caller's context while it waits
//   - 2026-09-21 09:46: settles through the scheduler
func _Abort(thread *starlark.Thread, handles []*scheduler.Handle) {
	for _, handle := range handles {
		handle.Stop()
	}

	for _, handle := range handles {
		scheduler.Settle(thread, handle)
	}
}
