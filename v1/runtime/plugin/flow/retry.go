package flow

import (
	"errors"
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ErrTimeout is returned when a bounded call runs out of time.
var ErrTimeout = errors.New("timed out")

// _Retry calls its target until one attempt succeeds.
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
func _Retry(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	target, attempts, err := _Counted(fn.Name(), args, kwargs)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s(%d, %s)", RETRY, attempts, target.Name())

	var last error

	for attempt := int32(ONCE); attempt <= attempts; attempt++ {
		value, err := _Attempted(thread, target, attempt, scheduler.Catching())
		if err == nil {
			_Finished(thread, target.Name(), nil)

			return value, nil
		}

		if !errors.Is(err, core.ErrAssert) {
			failed := fmt.Errorf("%s attempt %d: %w", name, attempt, err)

			_Finished(thread, target.Name(), failed)

			return nil, failed
		}

		last = err
	}

	exhausted := fmt.Errorf("%s gave up after %d attempts: %w", name, attempts, last)

	_Finished(thread, target.Name(), exhausted)

	return nil, scheduler.Fail(thread, exhausted)
}

// _Timeout gives its target a limited time to finish.
//
// The target runs beside the caller on the caller's own lane, because the
// caller has to be able to stop waiting. When the time runs out the evaluation
// is cancelled, and this waits for it to actually stop before returning: a
// cancelled evaluation stops at its next instruction, so returning as soon as
// the clock ran out would let it go on writing state after its caller had
// been told the call failed. Everything blocking inside it observes the same
// cancel, join included, so the wait is short.
//
// Revisions:
//   - 2026-09-20 01:21: initial creation
//   - 2026-09-21 08:09: Beside, inline; reads its budget through Duration
func _Timeout(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		given  starlark.Value
		target *starlark.Function
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &given, &target)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	budget, err := scheduler.Duration(given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	name := fmt.Sprintf("%s(%s, %s)", TIMEOUT, given.String(), target.Name())

	value, err := _Bounded(thread, target, budget)
	if err != nil {
		failed := fmt.Errorf("%s: %w", name, err)

		_Finished(thread, target.Name(), failed)

		return nil, failed
	}

	_Finished(thread, target.Name(), nil)

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
func _Bounded(
	thread *starlark.Thread,
	target *starlark.Function,
	budget time.Duration,
) (starlark.Value, error) {
	handle, err := scheduler.Beside(thread, target, scheduler.Inline())
	if err != nil {
		return nil, err
	}

	_Began(thread, target.Name(), scheduler.NO_ATTEMPT)

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

	return scheduler.Wait(thread, handle)
}
