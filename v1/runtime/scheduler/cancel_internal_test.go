// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: _CancelOn, the watch a thread keeps on its context, is unexported, and
// what its release promises is between it and the two places that start one.
package scheduler

import (
	"context"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

// TestCancelOn_ReleasesAgain checks the release _CancelOn returns can be
// called a second time and returns at once, whether the context was cancelled
// before the first or never was.
//
// Revisions:
//   - 2026-10-03 23:37: initial creation
func TestCancelOn_ReleasesAgain(t *testing.T) {
	cases := []struct {
		name      string
		cancelled bool
	}{
		{"never cancelled", false},
		{"cancelled first", true},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			ctx, stop := context.WithCancel(t.Context())
			defer stop()

			release := _CancelOn(ctx, &starlark.Thread{})

			if item.cancelled {
				stop()
			}

			released := make(chan struct{})

			go func() {
				release()
				release()
				close(released)
			}()

			select {
			case <-released:
			case <-time.After(CANCEL_LIMIT):
				t.Fatal("a second release did not return")
			}
		})
	}
}
