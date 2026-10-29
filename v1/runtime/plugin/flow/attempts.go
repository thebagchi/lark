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
// name is the function the line reports, and builtin the wrapper that reports
// it. catching says an attempt's assertion fails only that attempt, which is
// what a retry is for; a repeat's ends the run.
type _Attempts struct {
	target   *starlark.Function
	name     string
	builtin  string
	count    int32
	delay    time.Duration
	catching bool
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
// Returns ErrCount for a count below one, and scheduler.ErrDuration for a
// delay that is not a whole number of milliseconds an int32 holds.
//
// Revisions:
//   - 2026-09-20 01:08: initial creation, as _Named
//   - 2026-09-21 08:09: named for what it reads, since it refuses no lambda
//   - 2026-10-02 00:59: reads an optional delay and the callee the dialect
//     passes, and returns the attempts rather than a count and a function
//   - 2026-10-29 00:23: builds no text for an error, which _Written does only
//     when one fails
func _Counted(
	who string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*_Attempts, error) {
	var (
		callee, rest = scheduler.Hidden(
			kwargs,
			spelling.CALLEE,
		)
		count, target, delay, err = _CountArgs(
			who,
			args,
			rest,
		)
	)
	if err != nil {
		return nil, err
	}

	return &_Attempts{
		target: target,
		name: scheduler.Named(
			target,
			callee,
		),
		builtin: who,
		count:   count,
		delay:   delay,
	}, nil
}

// _Written is the call as the script wrote it, for an error to name.
//
// Built here, on a failure, rather than once for every call: a call that
// succeeds never reads it, and formatting it was three of the allocations a
// retry made.
//
// Revisions:
//   - 2026-10-29 00:23: initial creation, from the field _Counted filled
func (a *_Attempts) _Written() string {
	return fmt.Sprintf(
		"%s(%d, %s)",
		a.builtin,
		a.count,
		a.target.Name(),
	)
}

// _CountArgs reads the count, the function, and the delay a retry was given.
//
// The delay is unpacked again with whatever came third, so a delay named as
// well as passed third is refused as one given twice.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:23: unpacks into the fields of one _Given, so the three
//     places it reads into cost one allocation rather than one each
func _CountArgs(
	who string,
	args starlark.Tuple,
	rest []starlark.Tuple,
) (int32, *starlark.Function, time.Duration, error) {
	var (
		given = &_Given{
			args:  args,
			rest:  rest,
			delay: starlark.MakeInt(0),
		}
		err = starlark.UnpackPositionalArgs(
			who,
			args,
			nil,
			WRAPPED,
			&given.count,
			&given.target,
			&given.delay,
		)
	)
	if err != nil {
		return 0, nil, 0, fmt.Errorf(
			"%s: %w",
			who,
			err,
		)
	}

	delay, err := _CountDelay(
		who,
		given,
	)
	if err != nil {
		return 0, nil, 0, err
	}

	return given.count, given.target, delay, nil
}

// _Given is what a repeat or a retry was passed, and what is read out of it:
// the positional arguments and the keywords, then the count, the function,
// and the delay as the script gave it.
//
// One struct for every place an unpack writes to. An unpack takes pointers
// through an interface, so the compiler moves whatever they point at to the
// heap. Fields of one struct move together, as one allocation.
type _Given struct {
	args   starlark.Tuple
	rest   []starlark.Tuple
	count  int32
	target *starlark.Function
	delay  starlark.Value
}

// _CountDelay reads the optional delay and refuses a count below one.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:23: reads the count and the delay off the _Given its
//     caller unpacked into, and writes the delay back there
func _CountDelay(
	who string,
	given *_Given,
) (time.Duration, error) {
	err := starlark.UnpackArgs(
		who,
		given.args[WRAPPED:],
		given.rest,
		DELAY+"?",
		&given.delay,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"%s: %w",
			who,
			err,
		)
	}

	if given.count < ONCE {
		return 0, fmt.Errorf(
			"%s got %d: %w",
			who,
			given.count,
			ErrCount,
		)
	}

	delay, err := scheduler.Duration(given.delay)
	if err != nil {
		return 0, fmt.Errorf(
			"%s: %w",
			who,
			err,
		)
	}

	return delay, nil
}

// _Attempted runs attempt n on the caller's thread, after waiting out the
// delay that comes before it.
//
// The line is reported before the attempt starts, so the lines the attempt
// reports on its caller's lane come after the line they belong to. Attempt 1
// opens the line and each later attempt advances it, so repeat(3, step) is
// one line that reaches 3.
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
//   - 2026-10-29 00:23: takes no options, since whether an attempt catches is
//     one of the attempts, and starts it through _Beside
//   - 2026-10-29 00:39: runs the attempt through _Within, on the caller's
//     thread, and so returns what the attempt produced without a wait
func (a *_Attempts) _Attempted(
	thread *starlark.Thread,
	attempt int32,
) (starlark.Value, error) {
	err := a._Pause(
		thread,
		attempt,
	)
	if err != nil {
		return nil, err
	}

	scheduler.Open(
		thread,
		&scheduler.Line{
			Name:    a.name,
			Builtin: a.builtin,
			Attempt: attempt,
			Count:   a.count,
		},
	)

	return a._Within(
		thread,
		attempt,
	)
}

// _Within runs attempt on the caller's thread, counted as that attempt, and
// catching an assertion when the attempts are a retry's.
//
// On the caller's thread, because the caller only waits for the attempt. A
// goroutine and a thread of its own were most of what an attempt cost.
//
// Two calls rather than one list of options joined with append. Within keeps
// none of its options, so options written in its call stay on the stack. An
// append moves the slice it grows and the attempt's option to the heap, which
// was two allocations for every attempt.
//
// Revisions:
//   - 2026-10-29 00:23: initial creation, as _Beside, lifted out of _Attempted
//   - 2026-10-29 00:39: renamed for what it calls, which runs the attempt on
//     the caller's thread rather than beside it
func (a *_Attempts) _Within(
	thread *starlark.Thread,
	attempt int32,
) (starlark.Value, error) {
	if a.catching {
		return scheduler.Within(
			thread,
			a.target,
			scheduler.Attempt(attempt),
			scheduler.Catching(),
		)
	}

	return scheduler.Within(
		thread,
		a.target,
		scheduler.Attempt(attempt),
	)
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
// Returns scheduler.ErrCancelled, wrapping the context's error, when the
// evaluation is cancelled first.
//
// Revisions:
//   - 2026-10-02 00:59: initial creation
func (a *_Attempts) _Pause(
	thread *starlark.Thread,
	attempt int32,
) error {
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
		return fmt.Errorf(
			"%w: %w",
			scheduler.ErrCancelled,
			ctx.Err(),
		)
	}
}
