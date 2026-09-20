package flow

import (
	"errors"
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/scheduler"
)

// ErrTimeout is returned when a bounded call runs out of time.
var ErrTimeout = errors.New("timed out")

const SECOND = float64(time.Second)

// _Retry returns a callable that calls its target until one attempt succeeds.
//
// It retries **only** an assertion. Anything else - a fail(), a cancelled
// handle, a builtin that refused - propagates at once and is not attempted
// again. That distinction is the reason this runtime has both assert and fail:
// one says "this check did not hold, which may be timing", the other says "this
// cannot work".
//
// An assertion normally ends the whole run. Inside here it ends only the
// attempt, because the thread each attempt runs on is marked as catching. An unhandled failure stops everything; a handled one does not, which is
// what an exception language would call catching without a script gaining try.
//
// After the last attempt the assertion propagates as it would have anyway, so a
// retry that never succeeds ends the run exactly as a bare assertion does.
//
// Revisions:
//   - 2026-09-20 01:16: initial creation
func _Retry(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	target, attempts, err := _Named(fn.Name(), args, kwargs)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s(%s, %d)", RETRY, target.Name(), attempts)

	return starlark.NewBuiltin(name, func(
		inner *starlark.Thread,
		self *starlark.Builtin,
		passed starlark.Tuple,
		named []starlark.Tuple,
	) (starlark.Value, error) {
		var last error

		for attempt := ONCE; attempt <= attempts; attempt++ {
			value, err := _Await(inner, target, attempt, true, passed, named)
			if err == nil {
				_Finished(inner, target.Name(), nil)

				return value, nil
			}

			if !errors.Is(err, scheduler.ErrAssert) {
				failed := fmt.Errorf("%s attempt %d: %w", self.Name(), attempt, err)

				_Finished(inner, target.Name(), failed)

				return nil, failed
			}

			last = err
		}

		exhausted := _Exhausted(inner, self, attempts, last)

		_Finished(inner, target.Name(), exhausted)

		return nil, exhausted
	}), nil
}

// _Exhausted reports the last assertion after every attempt has failed, and
// lets it end the run.
//
// The attempts were caught so that each could fail alone. The last one is not:
// a retry that never succeeded is a failure nothing handled, and must behave
// like the bare assertion it started as.
//
// Revisions:
//   - 2026-09-20 01:19: initial creation
func _Exhausted(
	thread *starlark.Thread,
	self *starlark.Builtin,
	attempts int,
	last error,
) error {
	failure := fmt.Errorf("%s gave up after %d attempts: %w", self.Name(), attempts, last)

	scheduler.End(thread, failure)

	return failure
}

// _Timeout returns a callable that gives its target a limited time to finish.
//
// The target runs on a goroutine and an interpreter thread of its own, because
// the caller has to be able to stop waiting - and a Starlark thread cannot be
// shared across goroutines. When the time runs out the child is cancelled, and
// this waits for it to actually stop before returning.
//
// Waiting is the part worth stating. A cancelled evaluation stops at its next
// instruction, so returning as soon as the clock ran out would let the child go
// on writing state after its caller had been told the call failed. Waiting
// costs nothing when the child is Starlark - a cancel reaches it at once - and
// is bounded by the same rule every blocking builtin here follows.
//
// Revisions:
//   - 2026-09-20 01:21: initial creation
func _Timeout(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		target *starlark.Function
		given  starlark.Value
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &target, &given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	// A number, not a float, so timeout(fn, 5) works as well as timeout(fn, 0.5).
	seconds, ok := starlark.AsFloat(given)
	if !ok {
		return nil, fmt.Errorf("%s got %s: %w", fn.Name(), given.Type(), ErrCount)
	}

	if target.Name() == LAMBDA {
		return nil, fmt.Errorf("%s got a lambda: %w", fn.Name(), ErrNotAName)
	}

	if seconds <= 0 {
		return nil, fmt.Errorf("%s got %g seconds: %w", fn.Name(), seconds, ErrCount)
	}

	name := fmt.Sprintf("%s(%s, %g)", TIMEOUT, target.Name(), seconds)

	return starlark.NewBuiltin(name, func(
		inner *starlark.Thread,
		self *starlark.Builtin,
		passed starlark.Tuple,
		named []starlark.Tuple,
	) (starlark.Value, error) {
		return _Bounded(inner, self, target, seconds, passed, named)
	}), nil
}

// _Bounded runs target beside the caller and gives it seconds to finish.
//
// Revisions:
//   - 2026-09-20 01:23: initial creation
//   - 2026-09-20 01:29: shares _Aside with repeat and retry, which differ from
//     this only in having no deadline to wait against
func _Bounded(
	thread *starlark.Thread,
	self *starlark.Builtin,
	target *starlark.Function,
	seconds float64,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	// UNCOUNTED, not ONCE: a timeout makes one call rather than attempts, and
	// its progress is time against a budget, which nothing here reports. A
	// timeout node is indistinguishable from a plain call, which is right - the
	// wrapping is the caller's business, not the function's.
	done, stop, err := _Aside(thread, target, UNCOUNTED, false, args, kwargs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", self.Name(), err)
	}

	defer stop()

	timer := time.NewTimer(time.Duration(seconds * SECOND))
	defer timer.Stop()

	select {
	case got := <-done:
		if got.err != nil {
			failed := fmt.Errorf("%s: %w", self.Name(), got.err)

			_Finished(thread, target.Name(), failed)

			return nil, failed
		}

		_Finished(thread, target.Name(), nil)

		return got.value, nil

	case <-timer.C:
		stop()

		// Waited for, not merely cancelled: a child that stops at its next
		// instruction could otherwise still write state after this returned.
		<-done

		expired := fmt.Errorf("%s: %w", self.Name(), ErrTimeout)

		_Finished(thread, target.Name(), expired)

		return nil, expired
	}
}
