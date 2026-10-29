package scheduler

import (
	"context"
	"sync"

	"go.starlark.net/starlark"
)

// STOPPED is what the interpreter is told when a run is stopped, and what it
// then puts in the error it raises.
//
// A fixed word rather than the context's own text. Passing ctx.Err().Error()
// here put "context canceled" in the interpreter's message, and the caller
// wraps the context's error as well so that it stays matchable - so the same
// three words arrived twice in one sentence.
const (
	STOPPED = "the run was stopped"
)

// _Watch is the watch a thread keeps on its context: the cancel the context
// starts when it is done, and the count of that cancel while it may still run.
//
// One struct with a method, rather than a channel and two closures: the
// struct and the method value it hands AfterFunc are its only objects.
type _Watch struct {
	thread  *starlark.Thread
	stop    func() bool
	pending sync.WaitGroup
}

// _CancelOn cancels thread when ctx is done, and returns the watch whose
// release stops watching.
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
// The cancel is counted before AfterFunc can start it. A release that stops
// the cancel before it starts takes the count down itself, since the cancel
// never will: a second release then finds no count and returns at once, where
// it would have waited on a cancel that never came.
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
//   - 2026-10-28 22:36: returns a watch, whose count replaces the channel and
//     whose methods replace the two closures
func _CancelOn(
	ctx context.Context,
	thread *starlark.Thread,
) *_Watch {
	watch := &_Watch{
		thread: thread,
	}

	watch.pending.Add(1)
	watch.stop = context.AfterFunc(
		ctx,
		watch._Cancel,
	)

	return watch
}

// _Release stops watching, and returns once a cancel already under way has
// finished.
//
// _CancelOn tells why the release waits.
//
// Revisions:
//   - 2026-10-28 22:36: initial creation, out of the closure _CancelOn returned
func (w *_Watch) _Release() {
	stopped := w.stop()
	if stopped {
		w.pending.Done()
	}

	w.pending.Wait()
}

// _Cancel tells the thread the run was stopped, and counts the cancel done.
//
// Revisions:
//   - 2026-10-28 22:36: initial creation, out of the closure _CancelOn handed
//     AfterFunc
func (w *_Watch) _Cancel() {
	defer w.pending.Done()

	w.thread.Cancel(STOPPED)
}
