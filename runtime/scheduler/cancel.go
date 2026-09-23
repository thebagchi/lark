package scheduler

import (
	"context"

	"go.starlark.net/starlark"
)

// STOPPED is what the interpreter is told when a run is stopped, and what it
// then puts in the error it raises.
//
// A fixed word rather than the context's own text. Passing ctx.Err().Error()
// here put "context canceled" in the interpreter's message, and the caller
// wraps the context's error as well so that it stays matchable - so the same
// three words arrived twice in one sentence.
const STOPPED = "the run was stopped"

// _CancelOn cancels thread when ctx is done, and returns the function that
// stops watching.
//
// Stopping is synchronous: it returns only once the watcher has exited, so a
// context cancelled after the release cannot cancel the thread. Without that,
// a watcher woken by the release and the cancellation at once may take either,
// which a test that asserted the thread survived was the first to see.
//
// Revisions:
//   - 2026-09-19 18:42: initial creation
//   - 2026-09-21 08:09: the release waits for the watcher to exit
//   - 2026-09-23 06:58: tells the interpreter a fixed reason, so the context's
//     own text is not repeated by whoever wraps it
func _CancelOn(ctx context.Context, thread *starlark.Thread) func() {
	var (
		done   = make(chan struct{})
		exited = make(chan struct{})
	)

	go func() {
		defer close(exited)

		select {
		case <-ctx.Done():
			thread.Cancel(STOPPED)
		case <-done:
		}
	}()

	return func() {
		close(done)

		<-exited
	}
}
