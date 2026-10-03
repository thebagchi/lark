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

// ERR_TIMEOUT is returned when a bounded call runs out of time.
var ERR_TIMEOUT = errors.New("timed out")

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
	attempts, err := _Counted(fn.Name(), args, kwargs)
	if err != nil {
		return nil, err
	}

	var last error

	for attempt := int32(ONCE); attempt <= attempts.count; attempt++ {
		value, failure := attempts._Attempted(thread, attempt, scheduler.Catching())
		if failure == nil {
			scheduler.Close(thread, attempts.name, nil)

			return value, nil
		}

		if !errors.Is(failure, core.ERR_ASSERT) {
			failed := fmt.Errorf(
				"%s attempt %d: %w",
				attempts.written,
				attempt,
				failure,
			)

			scheduler.Close(thread, attempts.name, failed)

			return nil, failed
		}

		last = failure
	}

	exhausted := fmt.Errorf(
		"%s gave up after %d attempts: %w",
		attempts.written,
		attempts.count,
		last,
	)

	scheduler.Close(thread, attempts.name, exhausted)

	return nil, scheduler.Fail(thread, exhausted)
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
	callee, rest := scheduler.Hidden(kwargs, spelling.CALLEE)

	var (
		given  starlark.Value
		target *starlark.Function
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, rest, WRAPPED, &given, &target)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	budget, err := scheduler.Duration(given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	name := scheduler.Named(target, callee)
	written := fmt.Sprintf("%s(%s, %s)", TIMEOUT, given.String(), target.Name())

	scheduler.Open(thread, &scheduler.Line{Name: name, Builtin: TIMEOUT})

	value, err := _Bounded(thread, target, budget)
	if err != nil {
		failed := fmt.Errorf("%s: %w", written, err)

		scheduler.Close(thread, name, failed)

		return nil, failed
	}

	scheduler.Close(thread, name, nil)

	return value, nil
}

// _Bounded runs target beside the caller and gives it budget to finish.
//
// Three ways out. The evaluation ends on its own and its result is returned.
// The clock runs out, the evaluation is stopped and waited for, and the
// failure is ERR_TIMEOUT. The caller's own evaluation is cancelled, which the
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
	handle, err := scheduler.Beside(thread, target, scheduler.Inline())
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

		return nil, ERR_TIMEOUT
	}

	return scheduler.Wait(thread, handle)
}
