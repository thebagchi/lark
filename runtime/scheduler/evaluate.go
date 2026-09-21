package scheduler

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/guard"
)

// Evaluate calls target on an interpreter thread of its own, as a run of its
// own, and returns what it produced once everything it spawned has stopped.
//
// The rules for what a run produced are this package's, and this is where
// they live. The run's outcome is the answer when there is one, whatever the
// call returned: a script that asserted in a thread nobody joined still
// failed, and a spine that returned a value while the run was being torn down
// did not succeed. A failure arriving with ctx already done wraps ctx's own
// error, so a caller who stopped the run can say that is why - the
// interpreter raises its cancellation as text, which nothing could match.
//
// The call is guarded, so a panicking builtin fails the run rather than the
// process. Never panics.
//
// Revisions:
//   - 2026-09-19 22:40: initial creation, as the body of artifact.Invoke
//   - 2026-09-21 09:46: moved here, where the rules it applies are declared
func Evaluate(ctx context.Context, name string, target starlark.Callable) (starlark.Value, error) {
	thread := &starlark.Thread{Name: name}

	finish := Begin(ctx, thread, name)

	var (
		result starlark.Value
		err    error
	)

	guard.WithRecover(
		&result,
		&err,
		func() (starlark.Value, error) {
			return starlark.Call(thread, target, nil, nil)
		},
	)

	// Ended before the outcome is read, not deferred. Ending a run waits for
	// every thread it started, and a thread still running is a thread that
	// can still assert.
	finish()

	outcome := Outcome(thread)
	if outcome != nil {
		return nil, outcome
	}

	if err == nil {
		return result, nil
	}

	if ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
		return nil, fmt.Errorf("%w: %w", err, ctx.Err())
	}

	return nil, err
}
