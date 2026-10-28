package flow

import (
	"errors"
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// ErrTimeout is returned when a bounded call runs out of time.
var (
	ErrTimeout = errors.New("timed out")
)

// _Retry calls its target until one attempt succeeds, waiting the delay
// between attempts when one is given.
//
// It retries **only** an assertion. Anything else - a fail(), a cancelled
// handle, a builtin that refused - propagates at once and is not attempted
// again. That distinction is the reason this runtime has both assert and fail:
// one says "this check did not hold, which may be timing", the other says "this
// cannot work".
//
// An assertion normally ends the whole run. Inside here it ends only the
// attempt, because each attempt runs as an evaluation that is catching. After
// the last attempt the assertion fails through the scheduler as any assertion
// does - so a retry that never succeeds ends the run exactly as a bare
// assertion would, and inside an outer retry it is one failed attempt of that.
//
// Revisions:
//   - 2026-09-20 01:16: initial creation
//   - 2026-09-21 08:09: each attempt is Beside, catching, on the caller's lane;
//     giving up fails through Fail rather than ending the run outright
//   - 2026-10-02 01:03: takes an optional delay, and reports its line through
//     the scheduler with the attempts it may make
func _Retry(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	attempts, err := _Counted(
		fn.Name(),
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return _Retried(
		thread,
		attempts,
	)
}

// _Retried runs each attempt until one succeeds or the count is spent.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _Retried(
	thread *starlark.Thread,
	attempts *_Attempts,
) (starlark.Value, error) {
	var last error

	for attempt := int32(ONCE); attempt <= attempts.count; attempt++ {
		value, failure := attempts._Attempted(
			thread,
			attempt,
			scheduler.Catching(),
		)
		if failure == nil {
			scheduler.Close(
				thread,
				attempts.name,
				nil,
			)

			return value, nil
		}

		if !errors.Is(
			failure,
			core.ErrAssert,
		) {
			return _RetryFail(
				thread,
				attempts,
				attempt,
				failure,
			)
		}

		last = failure
	}

	return _GaveUp(
		thread,
		attempts,
		last,
	)
}

// _RetryFail closes the line and returns a failure that was not an assert.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _RetryFail(
	thread *starlark.Thread,
	attempts *_Attempts,
	attempt int32,
	failure error,
) (starlark.Value, error) {
	failed := fmt.Errorf(
		"%s attempt %d: %w",
		attempts.written,
		attempt,
		failure,
	)

	scheduler.Close(
		thread,
		attempts.name,
		failed,
	)

	return nil, failed
}

// _GaveUp closes the line after every attempt asserted, and stops the run.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _GaveUp(
	thread *starlark.Thread,
	attempts *_Attempts,
	last error,
) (starlark.Value, error) {
	exhausted := fmt.Errorf(
		"%s gave up after %d attempts: %w",
		attempts.written,
		attempts.count,
		last,
	)

	scheduler.Close(
		thread,
		attempts.name,
		exhausted,
	)

	return nil, scheduler.Fail(
		thread,
		exhausted,
	)
}

// _Timeout gives its target a limited time to finish, in milliseconds.
//
// The target runs beside the caller on the caller's own lane, because the
// caller has to be able to stop waiting. When the time runs out the evaluation
// is cancelled, and this waits for it to actually stop before returning: a
// cancelled evaluation stops at its next instruction, so returning as soon as
// the clock ran out would let it go on writing state after its caller had
// been told the call failed. Everything blocking inside it observes the same
// cancel, join included, so the wait is short.
//
// The line is reported before the target starts, since the target reports its
// own lines on the same lane, and it names the function the dialect read off a
// lambda, as a spawn's does.
//
// Revisions:
//   - 2026-09-20 01:21: initial creation
//   - 2026-09-21 08:09: Beside, inline; reads its budget through Duration
//   - 2026-10-02 01:03: reads milliseconds, and reports its line through the
//     scheduler before the target starts, under the callee the dialect passes
func _Timeout(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		callee, rest = scheduler.Hidden(
			kwargs,
			spelling.CALLEE,
		)
		given, target, budget, err = _TimeoutArgs(
			fn,
			args,
			rest,
		)
	)
	if err != nil {
		return nil, err
	}

	return _TimeoutRun(
		thread,
		&_Limit{
			callee: callee,
			given:  given,
			target: target,
			budget: budget,
		},
	)
}

// _TimeoutArgs reads the duration and the function a timeout wraps.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _TimeoutArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	rest []starlark.Tuple,
) (starlark.Value, *starlark.Function, time.Duration, error) {
	var (
		given  starlark.Value
		target *starlark.Function
	)

	err := starlark.UnpackPositionalArgs(
		fn.Name(),
		args,
		rest,
		WRAPPED,
		&given,
		&target,
	)
	if err != nil {
		return nil, nil, 0, fmt.Errorf(
			"%s: %w",
			fn.Name(),
			err,
		)
	}

	budget, err := scheduler.Duration(given)
	if err != nil {
		return nil, nil, 0, fmt.Errorf(
			"%s: %w",
			fn.Name(),
			err,
		)
	}

	return given, target, budget, nil
}

// _Limit is one function a timeout is about to run.
type _Limit struct {
	callee string
	given  starlark.Value
	target *starlark.Function
	budget time.Duration
}

// _TimeoutRun opens the line, runs the function, and closes the line.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _TimeoutRun(
	thread *starlark.Thread,
	limit *_Limit,
) (starlark.Value, error) {
	name := scheduler.Named(
		limit.target,
		limit.callee,
	)

	scheduler.Open(
		thread,
		&scheduler.Line{
			Name:    name,
			Builtin: TIMEOUT,
		},
	)

	value, err := _Bounded(
		thread,
		limit.target,
		limit.budget,
	)
	if err != nil {
		err = fmt.Errorf(
			"%s(%s, %s): %w",
			TIMEOUT,
			limit.given.String(),
			limit.target.Name(),
			err,
		)
		scheduler.Close(
			thread,
			name,
			err,
		)

		return nil, err
	}

	scheduler.Close(
		thread,
		name,
		nil,
	)

	return value, nil
}

// _Bounded runs target beside the caller and gives it budget to finish.
//
// Three ways out. The evaluation ends on its own and its result is returned.
// The clock runs out, the evaluation is stopped and waited for, and the
// failure is ErrTimeout. The caller's own evaluation is cancelled, which the
// bounded one descends from, so it ends too and Wait says why.
//
// Revisions:
//   - 2026-09-20 01:23: initial creation
//   - 2026-09-21 08:09: Beside and Wait, taking a duration already read
//   - 2026-10-02 01:03: reports nothing, since the timeout opens its line
//     before this starts the target
func _Bounded(
	thread *starlark.Thread,
	target *starlark.Function,
	budget time.Duration,
) (starlark.Value, error) {
	handle, err := scheduler.Beside(
		thread,
		target,
		scheduler.Inline(),
	)
	if err != nil {
		return nil, err
	}

	ctx, err := scheduler.Context(thread)
	if err != nil {
		return nil, err
	}

	timer := time.NewTimer(budget)
	defer timer.Stop()

	select {
	case <-handle.Done():
	case <-ctx.Done():
	case <-timer.C:
		handle.Stop()

		<-handle.Done()

		return nil, ErrTimeout
	}

	return scheduler.Wait(
		thread,
		handle,
	)
}
