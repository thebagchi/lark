package scheduler

import (
	"context"

	"go.starlark.net/starlark"
)

// _CancelOn cancels thread when ctx is done, and returns the function that
// stops watching.
//
// Revisions:
//   - 2026-09-19 18:42: initial creation
func _CancelOn(ctx context.Context, thread *starlark.Thread) func() {
	done := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			thread.Cancel(ctx.Err().Error())
		case <-done:
		}
	}()

	return func() {
		close(done)
	}
}
