package scheduler

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// ErrCancelled separates "something stopped you" from "your code was wrong",
// because a caller acts on them differently.
//
// It covers both stops a caller can meet: a joined handle that was cancelled,
// which is a fact about one thread, and a run that was stopped from outside,
// which is the caller's own doing. One sentinel rather than two, decided
// 2026-09-23 06:58 - a host asking "did this finish on its own" wants one
// answer, and the two are told apart by what else the error carries.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 20:10: an alias of run.ErrCancelled (C91w5)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
var (
	ErrCancelled = run.ErrCancelled
)

// Fork starts target on a goroutine and an interpreter thread of its own,
// inheriting everything the caller's evaluation carries, and returns the
// handle that finds its result.
//
// spawn calls it with no options and gets a lane of its own, numbered as a
// child of the caller's; a timeout calls it with Inline and gets an evaluation
// on the caller's own lane. The new thread's context descends from the
// caller's evaluation, not from the run, so a timeout around the caller
// reaches it. An attempt of a repeat or a retry runs Within instead.
//
// A lane reports itself starting and ending; an evaluation on the caller's
// lane does not, because its wrapper reports once for however many attempts
// it makes.
//
// The call is guarded, and this is the place where that matters most: a panic
// on a goroutine cannot be recovered from outside it, so an unguarded call
// here would not fail the run - it would take the process down, host and all.
//
// A spawn that owns a lane is refused once the run is stopped, whatever stopped
// it: a failure, the caller's context, the memory watch. No thread starts and
// nothing is reported for it, so a stopped run does no new work. A graph shows
// the spawn that was not made from the script (Q18). An evaluation on the
// caller's lane is not refused: its context is the caller's, and the caller
// is stopped by then or soon.
//
// A host builtin that calls Fork is refused while a module's top level runs,
// as spawn and timeout are: the top level, a function it calls and a module it
// loads all run before main, and the new thread would read globals they still
// write. Call it from main, or from a function main calls (Q383, Q389).
//
// For that call Fork returns, before anything starts, an error that matches
// runtime.ErrInit (the same value as core.ErrInit and script.ErrInit). The
// text names the builtin that called Fork and ends with a hint; with no
// builtin running it gives the text without a name. Fork returns ErrRun when
// thread carries no run or target is nil, and ErrCancelled wrapping the run's
// context error for a spawn made after the run stopped. A nil option is
// skipped.
//
// Revisions:
//   - 2026-09-19 20:38: initial creation, as _Spawn's body
//   - 2026-09-21 08:09: one primitive for spawn and every wrapper, inheriting
//     the caller's evaluation and deriving from its context
//   - 2026-10-02 00:38: reports a spawn on the thread that wrote it, with the
//     child's id, its binding and the function it runs
//   - 2026-11-01 13:00: charges no thread cost, the budget being gone
//   - 2026-11-02 15:48: starts the spawn and its flight, and returns the handle
//     inside the spawn
//   - 2026-11-04 23:50: a nil target is ErrNoCallee, and a nil option is skipped
//     (C86 fix round 3)
//   - 2026-11-05 00:27: refuses a spawn that owns a lane once the run is
//     stopped (C45, Q18)
//   - 2026-11-07 20:10: forwards to run.Beside (C91w5)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
//   - 2026-11-12 17:20: refuses a call made while the module's top level runs (C128, B278, Q389)
//   - 2026-11-12 18:58: renamed from Beside (C129, Q396)
func Fork(
	thread *starlark.Thread,
	target starlark.Callable,
	opts ...common.ForkOption,
) (common.Handle, error) {
	return run.Fork(
		thread,
		target,
		opts...,
	)
}

// Labelled tells Fork what the thread a spawn starts runs, and the name the
// script bound the spawn to, for the line it reports.
//
// The function is the one a lambda calls, which the dialect read off the
// source: a lambda is only how a spawn passes arguments, and its own name says
// nothing. Empty when the lambda calls no function the script defines.
//
// A child that is nil has nothing to label, and the option does nothing.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation
//   - 2026-11-04 00:30: does nothing for a nil child (C54, B085)
//   - 2026-11-07 20:10: forwards to run.Labelled (C91w5)
//   - 2026-11-12 18:58: uses Fork and ForkOption, formerly Beside and BesideOption (C129, Q396)
func Labelled(
	name string,
	binding string,
) common.ForkOption {
	return run.Labelled(
		name,
		binding,
	)
}

// Named is the name a line reports for a call of target: the callee the
// dialect read off the source, or target's own name, except that a lambda with
// no callee reports none, since "lambda" would read like a function of that
// name.
//
// One rule for every builtin that runs a function it was handed. A lambda is
// only how a spawn or a wrapper passes arguments, so the function it calls is
// what its line names.
//
// A nil target has no name of its own, so the callee is all there is.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation, as _Named
//   - 2026-10-02 00:59: exported, taking the callee rather than the locals, so
//     a wrapper names its line the way a spawn does
//   - 2026-11-04 00:30: a nil target names nothing, so the callee stands (C54, B086)
//   - 2026-11-04 23:50: names a nil target by its callee alone (C86 fix round 3)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:10: forwards to run.Named (C91w5)
func Named(
	target starlark.Callable,
	callee string,
) string {
	return run.Named(
		target,
		callee,
	)
}

// Attempt marks the evaluation as attempt n of a repeat or a retry, on the
// caller's own lane: an attempt is the caller's step, not a thread of its own.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-07 20:10: forwards to run.Attempt (C91w5)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func Attempt(n int32) common.ForkOption {
	return run.Attempt(n)
}

// Inline keeps the evaluation on the caller's lane without counting it as an
// attempt, which is what a timeout wants: one call, bounded, not numbered.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-07 20:10: forwards to run.Inline (C91w5)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func Inline() common.ForkOption {
	return run.Inline()
}

// Catching marks the evaluation as catching assertions, one level deeper than
// the caller's.
//
// An assertion stops the run when nothing catches it, and fails only the
// attempt when something does. That is what lets retry exist without a script
// gaining try or except. A depth rather than a flag, so a retry inside a retry
// is still inside one when the inner finishes.
//
// Revisions:
//   - 2026-09-20 01:00: initial creation, as Catch
//   - 2026-09-21 08:09: an option on Beside rather than a mark set afterwards
//   - 2026-11-07 20:10: forwards to run.Catching (C91w5)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func Catching() common.ForkOption {
	return run.Catching()
}

// Wait blocks until handle is over, or until the caller's own evaluation is
// cancelled, and returns what the handle produced.
//
// Waiting on the caller's context is what every blocking builtin owes. A join
// blocked on a handle from another lineage would otherwise hold a cancelled
// evaluation open for as long as that handle ran, which is how a timeout
// around a join used to wait the join out.
//
// Returns ErrCancelled wrapping the context's error when the caller is
// cancelled first.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-04 23:50: a nil or zero handle is over, so it answers nothing at once
//     (C86 fix round 3)
//   - 2026-11-07 20:10: forwards to run.Wait (C91w5)
func Wait(
	thread *starlark.Thread,
	handle common.Handle,
) (starlark.Value, error) {
	return run.Wait(
		thread,
		handle,
	)
}

// Settle waits for handle to be over, or for the caller's evaluation to be
// cancelled, and says nothing about either.
//
// For a caller that has already stopped the handle and only needs it gone
// before returning: a cancelled handle has nothing to say, so there is no
// result to discard.
//
// A nil handle is over already, so nothing is waited for.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation, as _Settle for join's abort
//   - 2026-09-21 09:46: exported, since join lives in a plugin now
//   - 2026-11-04 00:30: a nil handle returns at once (C54, B100)
//   - 2026-11-04 23:50: a nil handle is over, so there is nothing to wait for (C86 fix round 3)
//   - 2026-11-07 20:10: forwards to run.Settle (C91w5)
func Settle(
	thread *starlark.Thread,
	handle common.Handle,
) {
	run.Settle(
		thread,
		handle,
	)
}
