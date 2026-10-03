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
// Stopping is synchronous: it returns only once a cancel already under way has
// finished, so a context cancelled after the release cannot cancel the thread.
// Without that, a watcher woken by the release and the cancellation at once may
// take either, which a test that asserted the thread survived was the first to
// see. context.AfterFunc's own stop does not wait, so the release waits on the
// cancel when the stop reports it has already started.
//
// AfterFunc rather than a goroutine of its own: nothing runs until ctx is done,
// where a watcher was a goroutine and two channels for every thread.
//
// A release that stops the cancel before it starts closes the channel itself,
// since the cancel never will: a second release then finds it closed and
// returns at once, where it would have waited on a cancel that never came.
//
// Revisions:
//   - 2026-09-19 18:42: initial creation
//   - 2026-09-21 08:09: the release waits for the watcher to exit
//   - 2026-09-23 06:58: tells the interpreter a fixed reason, so the context's
//     own text is not repeated by whoever wraps it
//   - 2026-10-03 20:53: through context.AfterFunc, which starts nothing until ctx is
//     done, the release still waiting for a cancel already under way
//   - 2026-10-03 23:38: a second release returns at once, the first closing the channel
//     when it stopped the cancel
func _CancelOn(ctx context.Context, thread *starlark.Thread) func() {
	cancelled := make(chan struct{})

	stop := context.AfterFunc(ctx, func() {
		defer close(cancelled)

		thread.Cancel(STOPPED)
	})

	return func() {
		stopped := stop()
		if stopped {
			close(cancelled)

			return
		}

		<-cancelled
	}
}
