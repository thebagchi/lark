package run

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/guard"
)

// Eval runs body on an interpreter thread of its own, as the spine of a
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
// A failure arriving with ctx already done is ErrCancelled, wrapping what the
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
//   - 2026-09-23 06:58: a run stopped while running reports ErrCancelled, so a
//     caller tests one sentinel rather than the context error alone
//   - 2026-10-02 01:26: runs a body handed the run's thread, rather than
//     calling a target, so what precedes the entry point runs on the spine
//   - 2026-10-02 12:19: hands the body's error to the end of the run, so the
//     spine's line ends with what the run returns
//   - 2026-10-03 08:31: takes the run's settings, which it hands to Begin
//   - 2026-10-03 20:53: returns what the end of the run answers with, rather than
//     working it out again
//   - 2026-11-04 23:50: refuses a nil context with ErrNoRun (C86 fix round 3)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun; renamed from Evaluate
//     (C121, Q349, Q350)
func Eval(
	ctx context.Context,
	name string,
	body func(thread *starlark.Thread) (starlark.Value, error),
	settings *common.Settings,
) (starlark.Value, error) {
	if ctx == nil {
		return nil, fmt.Errorf(
			"no context to run in: %w",
			ErrRun,
		)
	}

	var (
		thread = &starlark.Thread{
			Name: name,
		}
		finish = Begin(
			ctx,
			thread,
			name,
			settings,
		)
	)

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

// _Placed is outcome with the place it arose, when the spine's own error holds
// it and says where.
//
// An outcome is recorded by Fail as it was raised, before the interpreter wraps
// it into an error with a stack; the spine's error is that wrapper. Where the
// wrapper holds the outcome, the wrapper knows the place. The outcome keeps its
// own text, which a builtin may have added to on the way up.
//
// Revisions:
//   - 2026-11-04 07:10: initial creation (C69)
//   - 2026-11-08 19:23: copies says through _Says and _Say (C112, Q207)
//   - 2026-11-10 01:00: now uses LocatedError, Where, Err, Say, and 1 more, were _LocatedError,
//     where, err, _Say, and 1 more (C120a, Q314)
func _Placed(
	outcome error,
	err error,
) error {
	var (
		have    *LocatedError
		located *LocatedError
		raised  = _Locate(
			err,
			nil,
		)
	)

	if errors.As(
		outcome,
		&have,
	) || !errors.Is(
		raised,
		outcome,
	) || !errors.As(
		raised,
		&located,
	) {
		return outcome
	}

	relocated := &LocatedError{
		Where: located.Where,
		Err:   outcome,
	}
	relocated.Say(located.Says())

	return relocated
}

// Produced is what a run produced, as an error: its outcome when it has one,
// whatever the evaluation returned, and otherwise the evaluation's own error,
// marked cancelled when ctx was already done.
//
// Applied once, by the end of the run, which ends the spine's own line with it
// and hands it back for Eval to return, so a graph cannot disagree with the
// error a host was handed.
//
// Revisions:
//   - 2026-10-02 12:19: initial creation, from Evaluate's body, so the end of
//     the run applies the same rule
//   - 2026-10-03 20:53: applied by the end of the run alone, which answers with it
//   - 2026-11-03 21:57: a nil ctx says nothing was stopped, so err comes back as it is
//     (C52, B061)
//   - 2026-11-04 06:39: an error that was not a stop carries the place it arose
//     (C69)
//   - 2026-11-04 07:10: an outcome the spine's error holds takes the spine's place
//     (C69)
//   - 2026-11-05 16:44: names the context's error once: an error that holds it
//     already (a sleep cut short) is not given it again (C82, B114)
//   - 2026-11-10 00:25: renamed from _Produced, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
func Produced(
	ctx context.Context,
	outcome error,
	err error,
) error {
	if outcome != nil {
		return _Placed(
			outcome,
			err,
		)
	}

	if err == nil {
		return nil
	}

	// Stopped, and the error does not say so yet: the interpreter raises its
	// cancellation as text, which nothing could match.
	stopped := ctx != nil && ctx.Err() != nil && !errors.Is(
		err,
		ErrCancelled,
	)
	if !stopped {
		return _Locate(
			err,
			nil,
		)
	}

	// An error that already wraps the context's error (a sleep cut short does)
	// keeps it matchable, and adding it again repeated its text.
	if errors.Is(
		err,
		ctx.Err(),
	) {
		return fmt.Errorf(
			"%w: %w",
			ErrCancelled,
			err,
		)
	}

	return fmt.Errorf(
		"%w: %w: %w",
		ErrCancelled,
		err,
		ctx.Err(),
	)
}
