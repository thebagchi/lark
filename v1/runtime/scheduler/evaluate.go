package scheduler

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/guard"
)

// Evaluate runs body on an interpreter thread of its own, as the spine of a
// run of its own, and returns what it produced once everything it spawned has
// stopped.
//
// body is handed the run's thread rather than a function to call, because
// whatever runs before the entry point - a script's module-level statements -
// belongs to the run too: a spawn there needs a run to join, and a call there
// is a line of the spine.
//
// The rules for what a run produced are this package's, and this is where
// they live. The run's outcome is the answer when there is one, whatever the
// call returned: a script that asserted in a thread nobody joined still
// failed, and a spine that returned a value while the run was being torn down
// did not succeed.
//
// A failure arriving with ctx already done is ERR_CANCELLED, wrapping what the
// interpreter said and then ctx's own error. The interpreter raises its
// cancellation as text, which nothing could match; the sentinel is what a
// caller tests, the text says where it stopped, and the context's error says
// it was a stop rather than a deadline.
//
// The spine's own line ends with that same answer, so a graph says the entry
// point failed when the run returns its failure.
//
// body is guarded, so a panicking builtin fails the run rather than the
// process. Never panics.
//
// Revisions:
//   - 2026-09-19 22:40: initial creation, as the body of artifact.Invoke
//   - 2026-09-21 09:46: moved here, where the rules it applies are declared
//   - 2026-09-23 06:58: a run stopped while running reports ERR_CANCELLED, so a
//     caller tests one sentinel rather than the context error alone
//   - 2026-10-02 01:26: runs a body handed the run's thread, rather than
//     calling a target, so what precedes the entry point runs on the spine
//   - 2026-10-02 12:19: hands the body's error to the end of the run, so the
//     spine's line ends with what the run returns
//   - 2026-10-03 08:31: takes the run's settings, which it hands to Begin
//   - 2026-10-03 20:53: returns what the end of the run answers with, rather than
//     working it out again
func Evaluate(
	ctx context.Context,
	name string,
	body func(thread *starlark.Thread) (starlark.Value, error),
	settings *Settings,
) (starlark.Value, error) {
	thread := &starlark.Thread{Name: name}

	finish := Begin(ctx, thread, name, settings)

	var (
		result starlark.Value
		err    error
	)

	guard.WithRecover(
		&result,
		&err,
		func() (starlark.Value, error) {
			return body(thread)
		},
	)

	// Ended before the outcome is read, not deferred. Ending a run waits for
	// every thread it started, and a thread still running is a thread that
	// can still assert.
	produced := finish(err)
	if produced != nil {
		return nil, produced
	}

	return result, nil
}

// _Produced is what a run produced, as an error: its outcome when it has one,
// whatever the evaluation returned, and otherwise the evaluation's own error,
// marked cancelled when ctx was already done.
//
// Applied once, by the end of the run, which ends the spine's own line with it
// and hands it back for Evaluate to return, so a graph cannot disagree with the
// error a host was handed.
//
// Revisions:
//   - 2026-10-02 12:19: initial creation, from Evaluate's body, so the end of
//     the run applies the same rule
//   - 2026-10-03 20:53: applied by the end of the run alone, which answers with it
func _Produced(ctx context.Context, outcome error, err error) error {
	if outcome != nil {
		return outcome
	}

	if err == nil {
		return nil
	}

	// Stopped, and the error does not say so yet: the interpreter raises its
	// cancellation as text, which nothing could match.
	stopped := ctx.Err() != nil && !errors.Is(err, ERR_CANCELLED)
	if !stopped {
		return err
	}

	return fmt.Errorf("%w: %w: %w", ERR_CANCELLED, err, ctx.Err())
}
