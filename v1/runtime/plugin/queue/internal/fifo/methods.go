package fifo

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// Values returns the queue module. Package queue hands it to the registry.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408, Q415, Q417)
func Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.NEW: starlark.NewBuiltin(
					spec.NAME+"."+spec.NEW,
					NewQueue,
				),
				spec.MEMBER_FULL:   starlark.String(spec.FULL),
				spec.MEMBER_CLOSED: starlark.String(spec.CLOSED),
			},
		},
	}
}

// NewQueue is queue.new: an empty queue, of the capacity given or of
// spec.CAPACITY when none is given or it is 0.
//
// A capacity that is not an int, or is negative, stops the whole run, the way
// a failed assertion does, rather than only the thread: a queue made wrong in
// a thread nobody joins would otherwise fail without a sound.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q411, Q418)
func NewQueue(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var given starlark.Value = starlark.MakeInt(0)

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		0,
		&given,
	)
	if err != nil {
		return nil, err
	}

	capacity, err := _Capacity(given)
	if err != nil {
		return nil, scheduler.Fail(
			thread,
			fmt.Errorf(
				"%s: %w",
				shared.Called(fn),
				err,
			),
		)
	}

	made, err := New(capacity)
	if err != nil {
		return nil, scheduler.Fail(
			thread,
			fmt.Errorf(
				"%s: %w",
				shared.Called(fn),
				err,
			),
		)
	}

	return made, nil
}

// _Capacity reads the capacity a script gave: an int that fits an int32.
//
// Names the type of what it refuses and never the value (Q4).
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q411, Q418)
func _Capacity(given starlark.Value) (int, error) {
	number, ok := given.(starlark.Int)
	if !ok {
		return 0, fmt.Errorf(
			"capacity of type %s: %w",
			given.Type(),
			unpack.ErrConvert,
		)
	}

	capacity, err := starlark.AsInt32(number)
	if err != nil {
		return 0, fmt.Errorf(
			"capacity does not fit an int32: %w",
			unpack.ErrRange,
		)
	}

	return capacity, nil
}

// _Push is q.push(value): it freezes value and adds it at the back.
//
// Answers None, or queue.FULL when the queue holds as many items as its
// capacity. Never waits. Freezes the value in place and keeps that same value,
// with no copy (Q420), so a value pushed to a full queue is frozen too.
//
// A value that is not data stops the whole run with unpack.ErrConvert, and so
// does a push to a closed queue with ErrClosed, for the reason an event.post
// that fails does: a thread nobody joins fails without a sound.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q409, Q412, Q413, Q420)
func _Push(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var value starlark.Value

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&value,
	)
	if err != nil {
		return nil, err
	}

	err = _Data(
		thread,
		fn,
		value,
	)
	if err != nil {
		return nil, err
	}

	value.Freeze()

	answer, err := _Receiver(fn).Push(value)
	if errors.Is(
		err,
		ErrClosed,
	) {
		return nil, scheduler.Fail(
			thread,
			fmt.Errorf(
				"%s: %w",
				shared.Called(fn),
				err,
			),
		)
	}

	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return answer, nil
}

// _Data refuses a value that is not data, and stops the run.
//
// A walk of the value, not a copy of it.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q413, Q420)
func _Data(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	value starlark.Value,
) error {
	bad, ok, err := deep.IsData(
		scheduler.Ambient(thread),
		value,
	)
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	if ok {
		return nil
	}

	return scheduler.Fail(
		thread,
		fmt.Errorf(
			"%s: %s: not data a queue can carry: %w",
			shared.Called(fn),
			bad.Type(),
			unpack.ErrConvert,
		),
	)
}

// _Pop is q.pop(): it takes the item at the front, and waits until there is
// one or the queue is closed.
//
// Answers the pair (value, None), or (None, queue.CLOSED) once the queue is
// closed and holds nothing. A pair, because None is a legal item. Takes no
// parameter: the wait ends with an item, a close, or the context of the caller,
// so a run cancel or a timeout() around the call reaches it.
//
// Raises rather than answering when the caller was cancelled, because then there
// is no script left to read the pair. Refuses inside a state.update with
// scheduler.ErrNested, as event.wait does. Hands out the item that was pushed,
// with no copy (Q420). Never panics.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q416, Q417, Q419, Q420)
func _Pop(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		0,
	)
	if err != nil {
		return nil, err
	}

	err = shared.NotUpdating(
		thread,
		fn,
	)
	if err != nil {
		return nil, err
	}

	ctx, err := scheduler.Context(thread)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	value, closed, err := _Receiver(fn).Pop(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	if closed {
		return starlark.Tuple{
			starlark.None,
			starlark.String(spec.CLOSED),
		}, nil
	}

	return starlark.Tuple{
		value,
		starlark.None,
	}, nil
}

// _Close is q.close(): no more items are accepted, and every waiting pop wakes.
//
// Answers None. A second close does nothing. Push and close never wait, so
// neither is refused inside an update.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q412)
func _Close(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		0,
	)
	if err != nil {
		return nil, err
	}

	err = _Receiver(fn).Close()
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return starlark.None, nil
}

// _Receiver is the queue a method was read from, or nil when fn is nil or
// was not bound to a queue. A method guards a nil queue itself.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q65, Q206)
func _Receiver(fn *starlark.Builtin) *Queue {
	if fn == nil {
		return nil
	}

	held, _ := fn.Receiver().(*Queue)

	return held
}
