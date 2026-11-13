package attempt

import (
	"errors"
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/plugin/flow/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// ErrTimeout is returned when a bounded call runs out of time.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow (C120b, Q315)
var (
	ErrTimeout = errors.New("timed out")
)

// Retry calls its target until one attempt succeeds, waiting the delay
// between attempts when one is given.
//
// It retries **only** an assertion or a value nested too deep (trail.ErrFatal),
// which end a run alike. Anything else - a fail(), a cancelled
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
//   - 2026-10-29 00:23: marks the attempts as catching, rather than handing
//     each attempt the option
//   - 2026-11-02 15:51: retries a value nested too deep as it retries an assertion (C26)
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _Retry to Retry, for the
//     tests in tests/ (C120b, Q315, Q314)
func Retry(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	attempts, err := _Counted(
		shared.Called(fn),
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	attempts.catching = true

	return Retried(
		thread,
		attempts,
	)
}

// Retried runs each attempt until one succeeds or the count is spent.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:23: hands an attempt no option, the attempts saying
//     themselves that they catch
//   - 2026-11-02 15:51: asks _Retryable in place of ErrAssert alone (C26)
//   - 2026-11-06 00:10: nil attempts are ErrNoCallee (C100b, B143)
//   - 2026-11-08 00:27: names read from internal/spec and common (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _Retried to Retried, for
//     the tests in tests/ (C120b, Q315, Q314)
func Retried(
	thread *starlark.Thread,
	attempts *Attempts,
) (starlark.Value, error) {
	if attempts == nil {
		return nil, _NoAttempts()
	}

	var last error

	for attempt := int32(spec.ONCE); attempt <= attempts.count; attempt++ {
		value, failure := attempts._Attempted(
			thread,
			attempt,
		)
		if failure == nil {
			scheduler.Close(
				thread,
				attempts.name,
				nil,
			)

			return value, nil
		}

		if !_Retryable(failure) {
			return RetryFail(
				thread,
				attempts,
				attempt,
				failure,
			)
		}

		last = failure
	}

	return GaveUp(
		thread,
		attempts,
		last,
	)
}

// _Retryable reports whether a failed attempt is one retry runs again: an
// assertion, or a value nested too deep.
//
// Two sentinels, asked here, rather than one shared sentinel both wrap. An
// assertion already ends the run through Fail, and making core.ErrAssert wrap
// trail.ErrFatal would send it through the end of a thread a second time for
// no change in what happens. trail.ErrFatal is the class of "ends the run
// unless something catches", which is what retry is, so asking it by name keeps
// the rule in the one place that retries.
//
// Revisions:
//   - 2026-11-02 15:51: initial creation (C26 fix round 1)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow (C120b, Q315)
func _Retryable(failure error) bool {
	return errors.Is(
		failure,
		core.ErrAssert,
	) || errors.Is(
		failure,
		trail.ErrFatal,
	)
}

// RetryFail closes the line and returns a failure that was not an assert.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:23: names the call through _Written, built only here
//   - 2026-11-06 00:10: nil attempts are ErrNoCallee (C100b, B143)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _RetryFail to RetryFail,
//     for the tests in tests/ (C120b, Q315, Q314)
func RetryFail(
	thread *starlark.Thread,
	attempts *Attempts,
	attempt int32,
	failure error,
) (starlark.Value, error) {
	if attempts == nil {
		return nil, _NoAttempts()
	}

	failed := fmt.Errorf(
		"%s attempt %d: %w",
		attempts._Written(),
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

// GaveUp closes the line after every attempt asserted, and stops the run.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:23: names the call through _Written, built only here
//   - 2026-11-06 00:10: nil attempts are ErrNoCallee (C100b, B144)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _GaveUp to GaveUp, for
//     the tests in tests/ (C120b, Q315, Q314)
func GaveUp(
	thread *starlark.Thread,
	attempts *Attempts,
	last error,
) (starlark.Value, error) {
	if attempts == nil {
		return nil, _NoAttempts()
	}

	exhausted := fmt.Errorf(
		"%s gave up after %d attempts: %w",
		attempts._Written(),
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

// Timeout gives its target a limited time to finish, in milliseconds.
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
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _Timeout to Timeout, for
//     the tests in tests/ (C120b, Q315, Q314)
func Timeout(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		callee, rest = scheduler.Hidden(
			kwargs,
			common.CALLEE,
		)
		given, target, budget, err = TimeoutArgs(
			fn,
			args,
			rest,
		)
	)
	if err != nil {
		return nil, err
	}

	return TimeoutRun(
		thread,
		&Limit{
			Callee: callee,
			Given:  given,
			target: target,
			budget: budget,
		},
	)
}

// TimeoutArgs reads the duration and the function a timeout wraps.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-08 00:27: names read from internal/spec and common (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _TimeoutArgs to
//     TimeoutArgs, for the tests in tests/ (C120b, Q315, Q314)
func TimeoutArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	rest []starlark.Tuple,
) (starlark.Value, *starlark.Function, time.Duration, error) {
	var (
		given  starlark.Value
		target *starlark.Function
	)

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		rest,
		spec.WRAPPED,
		&given,
		&target,
	)
	if err != nil {
		return nil, nil, 0, err
	}

	budget, err := scheduler.Duration(given)
	if err != nil {
		return nil, nil, 0, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return given, target, budget, nil
}

// Limit is one function a timeout is about to run.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _Limit to Limit; renamed
//     the fields callee to Callee, given to Given, for the tests in tests/ (C120b, Q315, Q314)
type Limit struct {
	Callee string
	Given  starlark.Value
	target *starlark.Function
	budget time.Duration
}

// TimeoutRun opens the line, runs the function, and closes the line.
//
// Returns script.ErrInit, before a line is opened, when thread is running the
// top level of a module: the function would run on a thread of its own while
// the top level still writes the globals it reads (Q389).
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-06 00:10: a nil limit, or one with no target, is ErrNoCallee before a line is
//     opened (C100b, B145, B146)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:27: names read from internal/spec and common (C91w12a)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _TimeoutRun to
//     TimeoutRun, for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
//   - 2026-11-12 06:29: refuses a timeout while the module's top level runs (C127, B274, Q389)
func TimeoutRun(
	thread *starlark.Thread,
	limit *Limit,
) (starlark.Value, error) {
	if limit == nil || limit.target == nil {
		return nil, fmt.Errorf(
			"%s: nil limit or target: %w",
			common.TIMEOUT,
			scheduler.ErrRun,
		)
	}

	_, loading := artifact.Supplied(thread)
	if loading {
		return nil, script.Loading(common.TIMEOUT)
	}

	name := scheduler.Named(
		limit.target,
		limit.Callee,
	)

	scheduler.Open(
		thread,
		&common.Line{
			Name:    name,
			Builtin: common.TIMEOUT,
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
			common.TIMEOUT,
			unpack.ShortText(limit.Given.String()),
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
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow (C120b, Q315)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func _Bounded(
	thread *starlark.Thread,
	target *starlark.Function,
	budget time.Duration,
) (starlark.Value, error) {
	handle, err := scheduler.Fork(
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
