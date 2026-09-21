package scheduler

import (
	"context"

	"go.starlark.net/starlark"
)

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
func _CancelOn(ctx context.Context, thread *starlark.Thread) func() {
	var (
		done   = make(chan struct{})
		exited = make(chan struct{})
	)

	go func() {
		defer close(exited)

		select {
		case <-ctx.Done():
			thread.Cancel(ctx.Err().Error())
		case <-done:
		}
	}()

	return func() {
		close(done)

		<-exited
	}
}
