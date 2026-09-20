package flow

import (
	"context"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/scheduler"
)

// _Outcome is what one attempt produced.
type _Outcome struct {
	value starlark.Value
	err   error
}

// _Aside starts target on a goroutine and an interpreter thread of its own, and
// returns the channel its result arrives on together with the function that
// stops it.
//
// Every wrapper here runs its target this way - repeat, retry and timeout
// alike. A timeout has to, because the caller must be able to stop waiting and
// a Starlark thread cannot be shared across goroutines. The other two do it for
// the same shape rather than because they must: an attempt is then one
// evaluation with one thread, whether something is bounding it or not, and the
// three wrappers differ only in how they wait.
//
// attempt is set on the new thread, not the caller's, so n() inside the target
// reads this attempt. catching marks the new thread too, which is what lets an
// assertion inside a retry end the attempt instead of the run.
//
// The caller must call the returned function, or the watcher the detached
// thread carries outlives it.
//
// Revisions:
//   - 2026-09-20 01:26: initial creation
func _Aside(
	thread *starlark.Thread,
	target *starlark.Function,
	attempt int,
	catching bool,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (<-chan _Outcome, context.CancelFunc, error) {
	beside, stop, err := scheduler.Detach(thread, target.Name())
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", target.Name(), err)
	}

	beside.SetLocal(ATTEMPT_KEY, attempt)

	_Began(thread, target.Name(), attempt)

	if catching {
		scheduler.Catch(beside)
	}

	done := make(chan _Outcome, 1)

	go func() {
		value, err := starlark.Call(beside, target, args, kwargs)

		done <- _Outcome{value: value, err: err}
	}()

	return done, stop, nil
}

// _Await runs one attempt beside the caller and waits for it.
//
// There is no deadline: a repeat or a retry waits as long as the attempt takes.
// Cancelling the run still reaches it, because the thread it runs on watches
// the run's context - which is the same rule sleep follows and the reason a
// cancelled run ends rather than hanging.
//
// Revisions:
//   - 2026-09-20 01:28: initial creation
func _Await(
	thread *starlark.Thread,
	target *starlark.Function,
	attempt int,
	catching bool,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	done, stop, err := _Aside(thread, target, attempt, catching, args, kwargs)
	if err != nil {
		return nil, err
	}

	defer stop()

	got := <-done

	return got.value, got.err
}
