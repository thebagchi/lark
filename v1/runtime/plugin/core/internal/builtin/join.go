package builtin

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ErrHandle is returned when join or cancel is given something that is not
// a handle, or any keyword argument.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
//   - 2026-11-10 17:15: renamed from ErrNotAHandle (C121, Q348)
var (
	ErrHandle = errors.New("wants handles")
)

// ErrSelf is returned when join is given the handle of the thread that
// calls it. Such a join would wait for itself for ever, so it fails at once
// (Q295). A wait across two threads is not refused.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation; proposed text (C116, B237, Q295)
//   - 2026-11-10 17:15: renamed from ErrSelfJoin (C121, Q348)
var (
	ErrSelf = errors.New("a thread cannot join its own handle")
)

// Wait is how a builtin that waits for handles answers once they are read:
// join raises the first failure, outcomes records each one.
//
// Revisions:
//   - 2026-11-02 17:03: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 20:07: takes common.Handle in place of *scheduler.Handle (C91w5, Q127)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Wait to Wait, for the
//     tests in tests/ (C120b, Q315, Q314)
type Wait func(
	thread *starlark.Thread,
	handles []common.Handle,
) (starlark.Value, error)

// Lined returns the function of a builtin that waits for handles. It reads
// the arguments as handles, reports the wait as a line of the thread that
// waits, and answers with what wait returns.
//
// It is the part join and outcomes share: one reading of the arguments, one
// line, one refusal inside an update. They differ only in wait. It returns a
// function, rather than taking wait beside the call, so that the call keeps
// the four arguments the linter allows.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation, from _Join's body
//   - 2026-11-06 06:45: names a nil builtin through the shared Called (C101, B182)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Lined to Lined, for the
//     tests in tests/ (C120b, Q315, Q314)
func Lined(wait Wait) func(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var (
			values starlark.Value

			handles, err = Handles(
				shared.Called(fn),
				args,
				kwargs,
			)
		)

		scheduler.Open(
			thread,
			&common.Line{
				Builtin: shared.Called(fn),
				Waits:   _Threads(handles),
			},
		)

		if err == nil {
			values, err = Joined(
				thread,
				fn,
				handles,
				wait,
			)
		}

		scheduler.Close(
			thread,
			"",
			err,
		)

		return values, err
	}
}

// Joined waits for every handle given, once Handles has read them, and
// returns what wait makes of them. It refuses first when this evaluation holds
// a name.
//
// Revisions:
//   - 2026-10-02 00:48: initial creation, from _Join's body, so the line _Join
//     reports closes whichever way this ends
//   - 2026-11-02 16:30: takes the wait, so outcomes shares it; the refusal of the
//     arguments is _Lined's
//   - 2026-11-03 21:57: a nil wait is scheduler.ErrNoCallee, and a nil thread
//     or builtin is _NotNested's error (C52, B068)
//   - 2026-11-06 00:20: names a nil builtin through the shared Called (C100b)
//   - 2026-11-07 20:07: takes common.Handle in place of *scheduler.Handle (C91w5, Q127)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Joined to Joined, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func Joined(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	handles []common.Handle,
	wait Wait,
) (starlark.Value, error) {
	err := NotNested(
		thread,
		fn,
	)
	if err != nil {
		return nil, err
	}

	if wait == nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			scheduler.ErrRun,
		)
	}

	return wait(
		thread,
		handles,
	)
}

// NotNested refuses a join while this evaluation holds a name.
//
// The wait can be one nothing in the script can end: an update takes the
// name before it calls its function. A thread that inherited the mark can
// join into the same cycle.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-03 21:57: names a nil builtin ANONYMOUS, and a nil thread is the
//     error scheduler.Updating gives (C52, B065)
//   - 2026-11-06 00:20: names a nil builtin through the shared Called (C100b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _NotNested to NotNested,
//     for the tests in tests/ (C120b, Q315, Q314)
func NotNested(
	thread *starlark.Thread,
	fn *starlark.Builtin,
) error {
	updating, err := scheduler.Updating(thread)
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	if updating == "" {
		return nil
	}

	return fmt.Errorf(
		"%s inside an update of %s: %w",
		shared.Called(fn),
		unpack.QuotePrefix(updating),
		scheduler.ErrNested,
	)
}

// Waited is what join answers with: it waits for every handle given and
// returns what each produced, in the order they were passed.
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
// Returns ErrHandle when an argument is something else, and
// scheduler.ErrCancelled when a joined handle was cancelled or the joining
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
//   - 2026-11-02 16:31: the answer of join alone; _Lined builds the call (C28)
//   - 2026-11-04 00:30: a nil handle is ErrNotAHandle, and the handles after it are
//     cancelled as for any failure (C54, B097)
//   - 2026-11-05 08:07: the failure of a handle comes from _Ended (C70)
//   - 2026-11-07 20:07: takes common.Handle in place of *scheduler.Handle (C91w5, Q127)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Waited to Waited, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:55: refuses the handle of the calling thread, before it waits for any
//     handle (C116, B237, Q295)
//   - 2026-11-10 17:15: uses ErrHandle, formerly ErrNotAHandle (C121, Q348)
func Waited(
	thread *starlark.Thread,
	handles []common.Handle,
) (starlark.Value, error) {
	err := _NotSelf(
		thread,
		handles,
	)
	if err != nil {
		return nil, err
	}

	values := make(
		[]starlark.Value,
		0,
		len(handles),
	)

	for index, handle := range handles {
		if handle == nil {
			Abort(
				thread,
				handles[index+1:],
			)

			return nil, fmt.Errorf(
				"%w: nil handle",
				ErrHandle,
			)
		}

		value, err := scheduler.Wait(
			thread,
			handle,
		)
		if err != nil {
			Abort(
				thread,
				handles[index+1:],
			)

			return nil, _Ended(
				thread,
				handle,
				err,
			)
		}

		values = append(
			values,
			value,
		)
	}

	return starlark.NewList(values), nil
}

// _NotSelf refuses a join of the handle of the thread that calls it, and
// does not look at the other handles.
//
// Only a handle a script got from spawn can reach a join, and a spawn is the
// owner of its lane, so a handle whose lane is the lane of the caller is the
// caller's own. A nil handle is left to Waited.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C116, B237, Q295)
//   - 2026-11-10 17:15: uses ErrSelf, formerly ErrSelfJoin (C121, Q348)
func _NotSelf(
	thread *starlark.Thread,
	handles []common.Handle,
) error {
	lane := scheduler.Lane(thread)

	for _, handle := range handles {
		if handle == nil || handle.Thread() != lane {
			continue
		}

		return fmt.Errorf(
			"%s: %w",
			handle.Name(),
			ErrSelf,
		)
	}

	return nil
}

// _Ended is the error of a join that handle ended, named for the thread.
// Convention (Q158): a hint follows the failing text as `<text> (<hint>)`, in lower case.
//
// A handle that was cancelled while the joining evaluation still runs gets
// CANCEL_HINT: someone called cancel on it, and outcomes() reads it without
// raising. A join that ends because the evaluation was stopped gets none,
// since reading the thread would not help.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation, out of _Waited (C70)
//   - 2026-11-07 20:07: takes common.Handle in place of *scheduler.Handle (C91w5, Q127)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func _Ended(
	thread *starlark.Thread,
	handle common.Handle,
	err error,
) error {
	if errors.Is(
		err,
		scheduler.ErrCancelled,
	) && scheduler.Ambient(thread).Err() == nil {
		return fmt.Errorf(
			"%s: %w (%s)",
			handle.Name(),
			err,
			spec.CANCEL_HINT,
		)
	}

	return fmt.Errorf(
		"%s: %w",
		handle.Name(),
		err,
	)
}

// _Threads is the thread each handle runs on, in order.
//
// Revisions:
//   - 2026-10-02 00:48: initial creation
//   - 2026-11-07 20:07: takes common.Handle in place of *scheduler.Handle (C91w5, Q127)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func _Threads(handles []common.Handle) []string {
	threads := make(
		[]string,
		0,
		len(handles),
	)

	for _, handle := range handles {
		threads = append(
			threads,
			handle.Thread(),
		)
	}

	return threads
}

// Handles reads a builtin's arguments as a list of handles.
//
// Revisions:
//   - 2026-09-19 20:45: initial creation
//   - 2026-11-04 00:30: a nil argument is ErrNotAHandle (C54, B099)
//   - 2026-11-07 20:07: takes common.Handle in place of *scheduler.Handle (C91w5, Q127)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Handles to Handles, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrHandle, formerly ErrNotAHandle (C121, Q348)
func Handles(
	name string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]common.Handle, error) {
	if len(kwargs) > 0 {
		return nil, fmt.Errorf(
			"%s takes no keyword arguments: %w",
			name,
			ErrHandle,
		)
	}

	handles := make(
		[]common.Handle,
		0,
		len(args),
	)

	for _, arg := range args {
		if arg == nil {
			return nil, fmt.Errorf(
				"%w: nil handle",
				ErrHandle,
			)
		}

		handle, ok := arg.(common.Handle)
		if !ok {
			return nil, fmt.Errorf(
				"%s got %s: %w",
				name,
				arg.Type(),
				ErrHandle,
			)
		}

		handles = append(
			handles,
			handle,
		)
	}

	return handles, nil
}

// Abort cancels every handle given and waits for each to stop, or for the
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
//   - 2026-11-04 00:30: a nil handle is skipped, by Stop and by Settle (C54, B100)
//   - 2026-11-07 20:07: takes common.Handle in place of *scheduler.Handle (C91w5, Q127)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Abort to Abort, for the
//     tests in tests/ (C120b, Q315, Q314)
func Abort(
	thread *starlark.Thread,
	handles []common.Handle,
) {
	for _, handle := range handles {
		if handle == nil {
			continue
		}

		handle.Stop()
	}

	for _, handle := range handles {
		scheduler.Settle(
			thread,
			handle,
		)
	}
}
