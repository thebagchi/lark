package tests

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// LOCKED is the name the lock tests take.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LOCKED = "counter"

	// ROUNDS is how many times the allocation test takes and releases the lock.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ROUNDS = 100

	// SETTLE is how long a waiter is given to take a held name, which it must
	// not do. LIMIT is how long a cancelled waiter may take to return.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SETTLE = 50 * time.Millisecond
	LIMIT  = 5 * time.Second

	// WAITING is how the error of a cancelled wait begins.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WAITING = `waiting for "counter"`
)

// TestRegression_LockAllocatesNothing is the cost a state loop paid on every
// update: Lock made a new release closure on each call, one allocation per
// get-and-set. A Hold is a value, and the lock is a slot that already exists
// after the first call, so taking and releasing it again must allocate nothing.
//
// Revisions:
//   - 2026-10-28 22:06: initial creation
//   - 2026-11-05 22:05: one allocation, the release of the hold (C58 fix round 1, Q85)
//   - 2026-11-05 22:40: back to none, with a Hold in place of a closure (C58 fix 2, Q201)
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRegression_LockAllocatesNothing(t *testing.T) {
	var (
		thread = &starlark.Thread{}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			nil,
		)
	)

	defer func() {
		err := finish(nil)
		if err != nil {
			t.Errorf(
				"finish: %v",
				err,
			)
		}
	}()

	var failure error

	allocs := testing.AllocsPerRun(
		ROUNDS,
		func() {
			release, err := scheduler.Lock(
				thread,
				LOCKED,
			)
			if err != nil {
				failure = err

				return
			}

			scheduler.Release(release)
		},
	)

	if failure != nil {
		t.Fatalf(
			"lock: %v",
			failure,
		)
	}

	if allocs != 0 {
		t.Fatalf(
			"a lock and its release made %v allocations, want 0",
			allocs,
		)
	}
}

// TestLock_AHeldNameWaitsUntilCancelled is the slow half of the lock: a name
// another evaluation holds is not taken, and the wait for it ends with
// ErrCancelled when the waiter is stopped. A free slot is taken without a look
// at the context, so only this test reaches the wait on the context.
//
// Revisions:
//   - 2026-10-28 22:06: initial creation
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestLock_AHeldNameWaitsUntilCancelled(t *testing.T) {
	var (
		thread = &starlark.Thread{}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			nil,
		)
		ready  = make(chan struct{})
		result = make(
			chan error,
			1,
		)
		waiter = starlark.NewBuiltin(
			"waiter",
			func(
				child *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				<-ready

				release, err := scheduler.Lock(
					child,
					LOCKED,
				)
				if err == nil {
					scheduler.Release(release)
				}

				result <- err

				return starlark.None, nil
			},
		)
	)

	defer func() {
		err := finish(nil)
		if err != nil {
			t.Errorf(
				"finish: %v",
				err,
			)
		}
	}()

	handle, err := scheduler.Fork(
		thread,
		waiter,
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	release, err := scheduler.Lock(
		thread,
		LOCKED,
	)
	if err != nil {
		t.Fatalf(
			"lock: %v",
			err,
		)
	}

	defer scheduler.Release(release)

	close(ready)

	select {
	case err = <-result:
		t.Fatalf(
			"the waiter returned %v while the name was held",
			err,
		)
	case <-time.After(SETTLE):
	}

	handle.Stop()

	select {
	case err = <-result:
	case <-time.After(LIMIT):
		t.Fatal("a stopped waiter still waits for the name")
	}

	var (
		cancelled = errors.Is(
			err,
			scheduler.ErrCancelled,
		) && errors.Is(
			err,
			context.Canceled,
		)
		named = strings.HasPrefix(
			err.Error(),
			WAITING,
		)
	)

	if !cancelled || !named {
		t.Fatalf(
			"got %v, want %s: ErrCancelled: context canceled",
			err,
			WAITING,
		)
	}
}

// TestLock_ASecondReleaseIsANoOp proves a release function called twice
// returns both times and leaves the name free, where the second call used to
// wait for ever on a slot it no longer held (B124, Q85).
//
// Revisions:
//   - 2026-11-05 19:07: initial creation (C58)
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLock_ASecondReleaseIsANoOp(t *testing.T) {
	var (
		thread = &starlark.Thread{}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			nil,
		)
	)

	defer func() {
		err := finish(nil)
		if err != nil {
			t.Errorf(
				"finish: %v",
				err,
			)
		}
	}()

	release, err := scheduler.Lock(
		thread,
		LOCKED,
	)
	if err != nil {
		t.Fatalf(
			"Lock: %v",
			err,
		)
	}

	returned := make(chan struct{})

	go func() {
		defer close(returned)

		scheduler.Release(release)
		scheduler.Release(release)
	}()

	select {
	case <-returned:
	case <-time.After(LIMIT):
		t.Fatal("a second release did not return")
	}

	again, err := scheduler.Lock(
		thread,
		LOCKED,
	)
	if err != nil {
		t.Fatalf(
			"Lock after two releases: %v",
			err,
		)
	}

	scheduler.Release(again)
}

// TestLock_AStaleReleaseLeavesTheNewHoldHeld proves a release function acts
// only on the hold it came from: Lock, release, Lock, then the first release
// again must leave the second hold held, so a further Lock is refused with
// ErrNested and Updating still names the name (C58 fix round 1, Q85).
//
// Revisions:
//   - 2026-11-05 19:40: initial creation (C58 fix round 1)
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLock_AStaleReleaseLeavesTheNewHoldHeld(t *testing.T) {
	var (
		thread     = _Begun(t)
		first, err = scheduler.Lock(
			thread,
			LOCKED,
		)
	)
	if err != nil {
		t.Fatalf(
			"first Lock: %v",
			err,
		)
	}

	scheduler.Release(first)

	second, err := scheduler.Lock(
		thread,
		LOCKED,
	)
	if err != nil {
		t.Fatalf(
			"second Lock: %v",
			err,
		)
	}

	scheduler.Release(first)

	held, err := scheduler.Updating(thread)
	if err != nil || held != LOCKED {
		t.Fatalf(
			"Updating = %q, %v, want %q after a stale release",
			held,
			err,
			LOCKED,
		)
	}

	_, err = scheduler.Lock(
		thread,
		OTHER,
	)
	if !errors.Is(
		err,
		scheduler.ErrNested,
	) {
		t.Fatalf(
			"Lock after a stale release gave %v, want ErrNested",
			err,
		)
	}

	scheduler.Release(second)
	scheduler.Release(second)

	third, err := scheduler.Lock(
		thread,
		OTHER,
	)
	if err != nil {
		t.Fatalf(
			"Lock after the current release: %v",
			err,
		)
	}

	scheduler.Release(third)
}

// TestLock_ConcurrentReleasesOfOneHoldFreeItOnce proves many goroutines
// calling one release together free the hold once, none waits, and the name
// can be taken again. It runs under -race.
//
// Revisions:
//   - 2026-11-05 19:40: initial creation (C58 fix round 1)
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLock_ConcurrentReleasesOfOneHoldFreeItOnce(t *testing.T) {
	const CALLERS = 16

	thread := _Begun(t)

	for range ROUNDS {
		release, err := scheduler.Lock(
			thread,
			LOCKED,
		)
		if err != nil {
			t.Fatalf(
				"Lock: %v",
				err,
			)
		}

		var group sync.WaitGroup

		for range CALLERS {
			group.Go(func() {
				scheduler.Release(release)
			})
		}

		done := make(chan struct{})

		go func() {
			group.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(LIMIT):
			t.Fatal("a concurrent release did not return")
		}
	}
}

// TestHold_AZeroHoldReleasesNothing proves the zero Hold, and the one Lock
// returns with an error, is safe to release, and leaves a real hold held.
//
// Revisions:
//   - 2026-11-05 22:40: initial creation (C58 fix 2, Q201, Q85)
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestHold_AZeroHoldReleasesNothing(t *testing.T) {
	var (
		thread = _Begun(t)
		zero   common.Hold
	)

	scheduler.Release(zero)

	held, err := scheduler.Lock(
		thread,
		LOCKED,
	)
	if err != nil {
		t.Fatalf(
			"Lock: %v",
			err,
		)
	}

	scheduler.Release(zero)

	_, err = scheduler.Lock(
		thread,
		OTHER,
	)
	if !errors.Is(
		err,
		scheduler.ErrNested,
	) {
		t.Fatalf(
			"Lock after a zero release gave %v, want ErrNested",
			err,
		)
	}

	scheduler.Release(held)

	_, err = scheduler.Lock(
		nil,
		LOCKED,
	)
	if !errors.Is(
		err,
		scheduler.ErrRun,
	) {
		t.Fatalf(
			"Lock on a nil thread gave %v, want ErrRun",
			err,
		)
	}
}

// TestLock_ALosingReleaseReturnsAfterTheWinnerFinished proves the promise of
// Release, "safe to call twice, from many goroutines together": when a call
// returns, the hold is freed, even if another goroutine freed it. So the owner
// that returns from its own Release can Lock again at once, with no write of
// the other call still to come. It runs under -race, which fails before the
// fix on the winner's late writes to the marks the next Lock reads.
//
// Revisions:
//   - 2026-11-10 11:48: initial creation, from the breaker case scheduler-F2 (C116, B236)
func TestLock_ALosingReleaseReturnsAfterTheWinnerFinished(t *testing.T) {
	const TRIES = 2000

	thread := _Begun(t)

	for round := range TRIES {
		hold, err := scheduler.Lock(
			thread,
			LOCKED,
		)
		if err != nil {
			t.Fatalf(
				"round %d: Lock: %v",
				round,
				err,
			)
		}

		var other sync.WaitGroup

		other.Go(func() {
			scheduler.Release(hold)
		})

		scheduler.Release(hold)

		again, err := scheduler.Lock(
			thread,
			LOCKED,
		)
		if err != nil {
			t.Fatalf(
				"round %d: Lock after Release returned: %v",
				round,
				err,
			)
		}

		other.Wait()
		scheduler.Release(again)
	}
}
