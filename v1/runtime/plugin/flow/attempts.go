package flow

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// _Attempts is what a repeat or a retry was asked to do: call target up to
// count times, waiting delay between calls, and report it as one line.
//
// name is the function the line reports, builtin the wrapper that reports it,
// and written the call as the script wrote it, for an error to name.
type _Attempts struct {
	target  *starlark.Function
	name    string
	builtin string
	written string
	count   int32
	delay   time.Duration
}

// _Counted reads a repeat's or a retry's arguments as the attempts it makes:
// a count, the function to call, and an optional delay in milliseconds.
//
// The count and the function by position only, and the delay third or by
// name, as in def repeat(count, function, /, delay = 0). That is the spelling
// derive reads, so a call accepted here is one derive models or keeps as text,
// never one it misreads.
//
// A lambda is accepted: a compiler emits one wherever a call passes arguments,
// since a wrapper takes none to pass on. The line names the function the
// dialect read off the lambda, as a spawn's does.
//
// Returns ERR_COUNT for a count below one, and scheduler.ERR_DURATION for a
// delay that is not a whole number of milliseconds an int32 holds.
//
// Revisions:
//   - 2026-09-20 01:08: initial creation, as _Named
//   - 2026-09-21 08:09: named for what it reads, since it refuses no lambda
//   - 2026-10-02 00:59: reads an optional delay and the callee the dialect
//     passes, and returns the attempts rather than a count and a function
func _Counted(
	who string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*_Attempts, error) {
	callee, rest := scheduler.Hidden(kwargs, spelling.CALLEE)

	var (
		count  int32
		target *starlark.Function
		given  starlark.Value = starlark.MakeInt(0)
	)

	err := starlark.UnpackPositionalArgs(who, args, nil, WRAPPED, &count, &target, &given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", who, err)
	}

	// Unpacked again with whatever came third, so a delay named as well as
	// passed third is refused as one given twice, and any other keyword as one
	// a wrapper does not take.
	err = starlark.UnpackArgs(who, args[WRAPPED:], rest, DELAY+"?", &given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", who, err)
	}

	if count < ONCE {
		return nil, fmt.Errorf("%s got %d: %w", who, count, ERR_COUNT)
	}

	delay, err := scheduler.Duration(given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", who, err)
	}

	return &_Attempts{
		target:  target,
		name:    scheduler.Named(target, callee),
		builtin: who,
		written: fmt.Sprintf("%s(%d, %s)", who, count, target.Name()),
		count:   count,
		delay:   delay,
	}, nil
}

// _Attempted runs attempt n beside the caller and waits for it, after waiting
// out the delay that comes before it.
//
// The line is reported before the attempt starts. An attempt runs on its
// caller's lane, so the lines it reports itself would otherwise race ahead of
// the line they belong to. Attempt 1 opens the line and each later attempt
// advances it, so repeat(3, step) is one line that reaches 3.
//
// The line is closed by the wrapper, once, not by an attempt. A caught failure
// is not the function failing - the whole point of retry is that attempt 2
// failing is ordinary - and closing the line on one would show a failed line
// that is about to be fine, and a host two failures that never happened.
//
// Revisions:
//   - 2026-09-20 01:28: initial creation, as _Await
//   - 2026-09-21 08:09: Beside and Wait, counted as an attempt, plus whatever
//     else a caller asks for
//   - 2026-10-02 00:59: a method of the attempts it is one of, waiting out the
//     delay first and reporting the line before the attempt starts
func (a *_Attempts) _Attempted(
	thread *starlark.Thread,
	attempt int32,
	opts ...scheduler.Option,
) (starlark.Value, error) {
	err := a._Pause(thread, attempt)
	if err != nil {
		return nil, err
	}

	scheduler.Open(thread, &scheduler.Line{
		Name:    a.name,
		Builtin: a.builtin,
		Attempt: attempt,
		Count:   a.count,
	})

	counted := append([]scheduler.Option{scheduler.Attempt(attempt)}, opts...)

	handle, err := scheduler.Beside(thread, a.target, counted...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.target.Name(), err)
	}

	return scheduler.Wait(thread, handle)
}

// _Pause waits out the delay that comes before attempt, or until the
// evaluation on thread is cancelled.
//
// Between calls only: nothing before the first attempt, and nothing after the
// last, since a repeat or a retry returns as soon as it is done. The wait is on
// the evaluation's own context rather than a call of sleep, so a cancel ends it
// before the next attempt starts, and the delay stays part of the wrapper's
// line rather than becoming a sleep line of its own.
//
// Returns scheduler.ERR_CANCELLED, wrapping the context's error, when the
// evaluation is cancelled first.
//
// Revisions:
//   - 2026-10-02 00:59: initial creation
func (a *_Attempts) _Pause(thread *starlark.Thread, attempt int32) error {
	waits := attempt > ONCE && a.delay > 0
	if !waits {
		return nil
	}

	ctx, err := scheduler.Context(thread)
	if err != nil {
		return err
	}

	timer := time.NewTimer(a.delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil

	case <-ctx.Done():
		return fmt.Errorf("%w: %w", scheduler.ERR_CANCELLED, ctx.Err())
	}
}
