package tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestGuard_BeginSharedThread proves Begin and the function it returns can be
// called from many goroutines on one thread without a data race. Run with
// -race.
//
// Case from Go-level breaker run 3, finding scheduler-F42 (bug B094), fix round
// 1. Lark calls Begin once per thread; this is the use of a caller that shares
// the thread anyway. The runs are not meaningful together (the last Begin owns
// the locals), but none of them may corrupt the thread.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54 fix round 1)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_BeginSharedThread(t *testing.T) {
	var (
		thread = new(starlark.Thread)
		group  sync.WaitGroup
	)

	for range 100 {
		group.Add(1)

		go func() {
			defer group.Done()

			finish := scheduler.Begin(
				context.Background(),
				thread,
				"test",
				new(common.Settings),
			)

			_ = finish(nil)
		}()
	}

	group.Wait()
}

// TestGuard_PollReadsHookOnce proves Poll calls a hook that clears itself, and
// calls it once.
//
// Case from Go-level breaker run 3, finding scheduler-F41 (bug B093). The
// breaker's own test set OnMaxSteps from a second goroutine; a starlark.Thread
// belongs to one goroutine, so that test is faulty and no guard in Poll can make
// it pass. Lark arms and releases the hook on the thread's own goroutine, so
// there is no race. This test covers what Poll can do: read the hook once.
//
// Revisions:
//   - 2026-11-04 06:55: initial creation in the external package, moved from the
//     internal file, as it uses only exported items (C54 fix round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_PollReadsHookOnce(t *testing.T) {
	var (
		thread = new(starlark.Thread)
		calls  int
	)

	thread.OnMaxSteps = func(self *starlark.Thread) {
		calls++
		self.OnMaxSteps = nil
	}

	scheduler.Poll(thread)
	scheduler.Poll(thread)

	if calls != 1 {
		t.Fatalf(
			"hook ran %d times, want 1",
			calls,
		)
	}
}

// TestGuard_BeginPanicReleasesGate proves a Begin that panics leaves the gate
// free: a normal Begin after it completes within a timeout.
//
// Found by the tester of C54: the gate was taken with no defer, so a Begin on a
// nil thread held it for good and every later Begin hung. Mutation check:
// removing the defer in _Seated makes this test time out.
//
// Revisions:
//   - 2026-11-04 06:55: initial creation (C54 fix round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_BeginPanicReleasesGate(t *testing.T) {
	func() {
		defer func() {
			_ = recover()
		}()

		_ = scheduler.Begin(
			context.Background(),
			nil,
			"test",
			nil,
		)
	}()

	finished := make(chan struct{})

	go func() {
		defer close(finished)

		finish := scheduler.Begin(
			context.Background(),
			new(starlark.Thread),
			"test",
			nil,
		)

		_ = finish(nil)
	}()

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Begin after a panicking Begin did not complete: the gate is held")
	}
}
