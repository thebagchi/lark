package scheduler_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// LOCKED is the name the lock tests take.
	LOCKED = "counter"

	// ROUNDS is how many times the allocation test takes and releases the lock.
	ROUNDS = 100

	// SETTLE is how long a waiter is given to take a held name, which it must
	// not do. LIMIT is how long a cancelled waiter may take to return.
	SETTLE = 50 * time.Millisecond
	LIMIT  = 5 * time.Second

	// WAITING is how the error of a cancelled wait begins.
	WAITING = `waiting for "counter"`
)

// TestRegression_LockAllocatesNothing is the cost a state loop paid on every
// update: Lock made a new release closure on each call, one allocation per
// get-and-set. The lock is a slot that already exists after the first call,
// so taking and releasing it again must allocate nothing.
//
// Revisions:
//   - 2026-10-28 22:06: initial creation
func TestRegression_LockAllocatesNothing(t *testing.T) {
	var (
		thread = &starlark.Thread{}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			spelling.ENTRY,
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

			release()
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
func TestLock_AHeldNameWaitsUntilCancelled(t *testing.T) {
	var (
		thread = &starlark.Thread{}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			spelling.ENTRY,
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
					release()
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

	handle, err := scheduler.Beside(
		thread,
		waiter,
	)
	if err != nil {
		t.Fatalf(
			"beside: %v",
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

	defer release()

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
