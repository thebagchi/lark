package scheduler

import (
	"context"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// ErrRun is returned when a builtin is called on a thread no run set up, and
// when a call is handed a nil object it needs: a nil callee, handle, walk or
// receiver. The wrap names what is missing.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 20:10: an alias of run.ErrNoRun (C91w5)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: renamed from ErrNoRun (C121, Q349)
var (
	ErrRun = run.ErrRun
)

// ErrLocal is returned when two plugins claim one key for values of
// different types.
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 20:10: an alias of run.ErrNotLocal (C91w5)
//   - 2026-11-10 17:15: renamed from ErrNotLocal (C121, Q349)
var (
	ErrLocal = run.ErrLocal
)

// Begin attaches a new run to thread, with settings, and returns the function
// that ends it.
//
// This is the whole of what another package needs from this one: a call that
// wants spawn, join and cancel to work on its thread calls Begin, evaluates,
// and hands what it returns the evaluation's error. settings says what watches
// the run, where it prints and what it may hold; nil is a zero Settings.
//
// That error is how the spine's own line ends, by the rule Eval returns:
// the run's outcome when it has one, and otherwise the error, cancelled when
// the run was stopped. Without it a spine whose entry point raised would end
// as succeeded, since raising is not an outcome - nothing else ends the run.
// The function answers with what that rule made of it, which is what the run
// produced.
//
// Ending a run cancels its context, which every evaluation's context descends
// from, and then waits for every goroutine it started. Cancelling first is the
// order that matters: a child that spawns while the run is being torn down
// derives from a context that is already cancelled, so nothing it starts can
// escape. Measured the other way round, a child that kept spawning held the
// run open on 200 of 200 runs.
//
// Revisions:
//   - 2026-09-19 22:39: initial creation
//   - 2026-09-20 01:39: takes the entry point's name and reports the spine
//     starting and stopping, since this is where thread 0 is set up
//   - 2026-09-21 08:09: cancels the run's context before waiting, rather than
//     each handle it knew of; leaves one local on the thread; sets what print
//     reaches
//   - 2026-09-21 09:46: print reaches the reporter, which is the one thing a
//     run tells its host
//   - 2026-10-02 00:38: reports the spine starting as a Line
//   - 2026-10-02 12:19: the function it returns takes the evaluation's error,
//     so the spine ends failed when its entry point raised
//   - 2026-10-02 13:12: print writes the transcript ctx carries, whether or not
//     anything reports
//   - 2026-10-02 16:21: print logs to the logger ctx carries, or to slog's
//     default
//   - 2026-10-03 08:31: takes the run's settings, rather than reading a reporter, a
//     logger and a ceiling off ctx
//   - 2026-10-03 20:53: the function it returns answers with what the run produced, so
//     the rule is applied once and its caller is handed the result
//   - 2026-11-01 13:00: settings carry no ceiling
//   - 2026-11-02 22:32: starts the memory watch when settings ask for one, and
//     stops it in the function it returns, after the wait (C27)
//   - 2026-11-04 06:55: changes the thread through _Seated, so two callers that share a thread
//     do not race, and a panic in it leaves the gate free (C54 fix rounds 1 and 2, B094)
//   - 2026-11-04 23:50: takes a nil context as Background and a nil thread as a new one, since a
//     function that returns only a finisher cannot refuse them (C86 fix round 3)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:10: forwards to run.Begin (C91w5)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
func Begin(
	ctx context.Context,
	thread *starlark.Thread,
	name string,
	settings *common.Settings,
) func(err error) error {
	return run.Begin(
		ctx,
		thread,
		name,
		settings,
	)
}

// Context returns the context of the evaluation on thread.
//
// A builtin that blocks needs it. Cancelling a run stops the interpreter
// between instructions, which does nothing to a Go call already waiting - so
// anything that waits must wait on this too, or a cancelled run hangs instead
// of ending. It is the evaluation's own context, not the run's: a bounded call
// is cancellable on its own, and something blocked inside it must observe that
// narrower cancel or the thing bounding it waits for the full duration anyway.
//
// Revisions:
//   - 2026-09-20 00:57: initial creation
//   - 2026-09-21 08:09: read from the one local
//   - 2026-11-07 20:10: forwards to run.Context (C91w5)
func Context(thread *starlark.Thread) (context.Context, error) {
	return run.Context(thread)
}

// Ambient returns the context of the evaluation on thread, or
// context.Background() when thread runs outside a run.
//
// For a walk that only has to stop when the run is cancelled: it has no error
// to give a thread that no run owns, and a Background context is one nothing
// cancels. A builtin that waits uses Context, which says when there is none.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-07 20:10: forwards to run.Ambient (C91w5)
func Ambient(thread *starlark.Thread) context.Context {
	return run.Ambient(thread)
}

// Fail records cause as why the run on thread ended and stops the run - unless
// something is catching assertions, in which case it returns cause and stops
// nothing.
//
// The one place that question is asked. assert calls it, and so does a retry
// that has given up; asking it in one caller and not the other is how an
// exhausted inner retry used to end the run under an outer one.
//
// A failure nothing catches stops everything: the spine, every spawned thread,
// and anything they spawned. That is what a test runner does - the first
// assertion ends the test. The outcome is recorded before anything is
// cancelled, so the run's answer is already fixed by the time the shutdown
// starts producing errors of its own.
//
// Returns cause unchanged either way, so a caller raises it.
//
// Revisions:
//   - 2026-09-20 00:09: initial creation, as _Stop
//   - 2026-09-20 01:20: End added beside it, for retry
//   - 2026-09-21 08:09: one exported function replacing both, so nothing can
//     end a run without the catching question being asked
//   - 2026-10-03 23:52: keeps cause on the evaluation as well, which then ends with
//     it, for a caller with nowhere to raise it
//   - 2026-11-02 12:28: ends the run through _Fail, which the end of a thread
//     shares (C26)
//   - 2026-11-03 21:57: a nil thread carries no run, so cause is returned and nothing
//     stops (C52, B059)
//   - 2026-11-07 20:10: forwards to run.Fail (C91w5)
func Fail(
	thread *starlark.Thread,
	cause error,
) error {
	return run.Fail(
		thread,
		cause,
	)
}

// AttemptOf is the attempt the evaluation on thread is counted as, or
// common.NO_ATTEMPT outside a repeat or a retry.
//
// Inherited: a thread spawned inside an attempt is inside that attempt, so an
// attempt's work can spawn and still ask which attempt it is.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:10: forwards to run.AttemptOf (C91w5)
func AttemptOf(thread *starlark.Thread) int32 {
	return run.AttemptOf(thread)
}

// Shared returns this run's value for key, building it the first time it is
// asked for.
//
// It is how a plugin holds something that belongs to one execution rather than
// to the process or to a compile - a store two spawned threads share, say. Two
// runs of one artifact get two values; the threads within a run get the same
// one, because they carry the same run.
//
// build is called at most once per run per key, so two threads racing to be
// first still see one value. It runs with the run's mutex released, under one
// mutex for that key: build may call Shared for another key or start a thread,
// and only another caller of the same key waits for it. A build that asks for
// its own key, directly or through others, waits for itself.
//
// The map is keyed to any because what a plugin stores is its own business and
// no two plugins store the same shape. The type parameter is what keeps that
// contained: a caller names the type it expects and never sees the assertion.
//
// Returns ErrRun when the thread has no run, and ErrLocal when key already
// holds something of another type - which means two plugins chose one key.
//
// Revisions:
//   - 2026-09-20 00:31: initial creation, as Local
//   - 2026-09-21 09:46: renamed: what a run shares is not what an evaluation
//     carries, and both were called locals
//   - 2026-10-29 01:05: finds the evaluation through EvaluationOf, and keeps
//     its body in SharedIn
//   - 2026-11-07 20:10: forwards to run.Shared (C91w5)
//   - 2026-11-10 11:52: build runs outside the run mutex, once for each key (C116, B238, Q296)
//   - 2026-11-10 17:15: uses ErrRun and ErrLocal, formerly ErrNoRun and ErrNotLocal (C121, Q349)
func Shared[T any](
	thread *starlark.Thread,
	key string,
	build func() T,
) (T, error) {
	return run.Shared(
		thread,
		key,
		build,
	)
}
