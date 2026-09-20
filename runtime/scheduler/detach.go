package scheduler

import (
	"context"
	"sync"

	"go.starlark.net/starlark"
)

const (
	// CATCH_KEY names the thread-local that marks an evaluation as catching.
	CATCH_KEY = "scheduler.catching"

	// CONTEXT_KEY names the thread-local holding an evaluation's own context.
	CONTEXT_KEY = "scheduler.context"
)

// Context returns the context of the run on thread.
//
// A builtin that blocks needs it. Cancelling a run stops the interpreter
// between instructions, which does nothing to a Go call already waiting - so
// anything that waits must wait on this too, or a cancelled run hangs instead
// of ending.
//
// Revisions:
//   - 2026-09-20 00:57: initial creation
func Context(thread *starlark.Thread) (context.Context, error) {
	// The evaluation's own context, not the run's. A detached evaluation - a
	// timeout's target, a spawned function - is cancellable on its own, and
	// something blocked inside it must observe that narrower cancel or the
	// thing bounding it waits for the full duration anyway.
	own, ok := thread.Local(CONTEXT_KEY).(context.Context)
	if ok {
		return own, nil
	}

	run, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	return run.ctx, nil
}

// Detach returns a thread and a context for an evaluation that runs beside the
// caller rather than under it, and the function that stops it.
//
// A Starlark thread cannot be shared across goroutines, so anything running
// concurrently needs one of its own - this is what spawn does, and what a
// timeout needs for the call it is bounding. The new thread carries the same
// run, so the work can still spawn, join and reach state.
//
// It is not tracked as a handle: nothing joins it, and the caller is
// responsible for stopping it. A timeout stops it when it runs out of time.
//
// Revisions:
//   - 2026-09-20 00:58: initial creation
func Detach(thread *starlark.Thread, name string) (*starlark.Thread, context.CancelFunc, error) {
	run, err := _Of(thread)
	if err != nil {
		return nil, nil, err
	}

	inner, stop := context.WithCancel(run.ctx)

	made := &starlark.Thread{Name: name}
	made.SetLocal(RUN_KEY, run)
	made.SetLocal(THREAD_KEY, thread.Local(THREAD_KEY))
	made.SetLocal(CONTEXT_KEY, inner)
	made.SetLocal(REPORTER_KEY, thread.Local(REPORTER_KEY))

	watching := _CancelOn(inner, made)

	// Idempotent, because a caller that stops early then stops again on the way
	// out is the ordinary shape - a timeout does exactly that - and releasing a
	// watcher twice closes a closed channel.
	var once sync.Once

	return made, func() {
		once.Do(func() {
			stop()
			watching()
		})
	}, nil
}

// Catching reports whether this evaluation is inside something that catches
// assertions.
//
// An assertion stops the run when nothing catches it, and fails only the
// attempt when something does. That is the difference between an unhandled
// failure and a handled one, and it is what lets retry exist without a script
// gaining try or except.
//
// Revisions:
//   - 2026-09-20 00:59: initial creation
func Catching(thread *starlark.Thread) bool {
	depth, ok := thread.Local(CATCH_KEY).(int)

	return ok && depth > 0
}

// Catch marks this evaluation as catching assertions and returns the function
// that stops.
//
// The mark is a depth rather than a flag, so a retry inside a retry restores
// the outer one rather than clearing it.
//
// Revisions:
//   - 2026-09-20 01:00: initial creation
func Catch(thread *starlark.Thread) func() {
	depth, _ := thread.Local(CATCH_KEY).(int)

	thread.SetLocal(CATCH_KEY, depth+1)

	return func() {
		thread.SetLocal(CATCH_KEY, depth)
	}
}
