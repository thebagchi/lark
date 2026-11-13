// This file tests Handle and the spawn against orders and values no spawn makes.
// The races call Keep, which stores a result, Detach, Hold, SetHook, SetDone and
// Chan, in an order no spawn makes (the handle done before the store). The C112
// tests call the Set methods, Keep, Hold, Report, Work, Watch and Locals on a
// Handle or a spawn, as breaker guard tests (Q206, Q207).
// TestGuard_C112_HandleZeroReads is in handle_test.go.
//
// Revisions:
//   - 2026-11-04 18:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:45: the header names the Q206 and Q207 decisions, and
//     TestGuard_C112_HandleZeroReads moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ from guard_race_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
package tests

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// TestGuard_ResultAgainstStore proves Result and the store of a result do not
// race, even when the store comes after done is closed. Run with -race.
//
// Case from Go-level breaker run 3, finding scheduler-F26 (bug B079). The
// breaker's own test wrote the fields with no lock, so no code in Handle could
// make it pass: the test is faulty. Lark itself stores the result before done
// closes, so it never raced. This test writes through Keep, the way lark does,
// and puts the store after the close to prove the guard holds in the worse
// order.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-08 19:23: sets done through _SetDone (C112, Q207)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_ResultAgainstStore(t *testing.T) {
	var (
		handle = new(run.Handle)
		group  sync.WaitGroup
	)

	handle.SetDone(make(chan struct{}))
	close(handle.Chan())

	group.Add(1)

	go func() {
		defer group.Done()

		for range 1000 {
			handle.Keep(
				starlark.String("race"),
				nil,
			)
		}
	}()

	for range 1000 {
		_, done, _ := handle.Result()
		if !done {
			t.Fatal("a closed handle is not done")
		}
	}

	group.Wait()
}

// TestGuard_StopAgainstDetach proves Stop and Detach do not race, which is the
// two ways the stop of a handle changes in lark. Run with -race.
//
// Case from Go-level breaker run 3, finding scheduler-F39 (bug B091). The
// breaker's own test wrote h.stop with no lock; Stop already reads it under the
// mutex, so no change to Handle could make that test pass: the test is faulty.
// This test drives the write lark makes.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_StopAgainstDetach(t *testing.T) {
	var (
		handle = &run.Handle{
			Cancel: func() {
				// Empty
			},
		}
		group sync.WaitGroup
	)

	group.Add(1)

	go func() {
		defer group.Done()

		for range 1000 {
			handle.Detach()()
		}
	}()

	for range 1000 {
		handle.Stop()
	}

	group.Wait()
}

// TestGuard_HoldAgainstStop proves the write lark makes to stop (Hold) does not
// race Stop or Detach. Run with -race.
//
// Case from Go-level breaker run 3, finding scheduler-F39 (bug B091), fix round
// 1: the breaker's own write is a bare assignment of the field, which no code
// in Handle can order; lark's own writes are all under the mutex.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_HoldAgainstStop(t *testing.T) {
	var (
		handle = new(run.Handle)
		group  sync.WaitGroup
	)

	group.Add(2)

	go func() {
		defer group.Done()

		for range 1000 {
			handle.Hold(func() {
				// Empty
			})
		}
	}()

	go func() {
		defer group.Done()

		for range 1000 {
			handle.Detach()()
		}
	}()

	for range 1000 {
		handle.Stop()
	}

	group.Wait()
}

// TestGuard_PollAgainstSetHook proves Poll and the hook lark sets from another
// goroutine do not race. Run with -race.
//
// Case from Go-level breaker run 3, finding scheduler-F41 (bug B093), fix round
// 1: the breaker's own write is a bare assignment of OnMaxSteps, a field of
// starlark.Thread that no code of lark can order; lark's own writes
// (Arm and Release) go through SetHook, and Poll reads through _Hook.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_PollAgainstSetHook(t *testing.T) {
	var (
		thread = new(starlark.Thread)
		group  sync.WaitGroup
		hook   = func(*starlark.Thread) {
			// Empty
		}
	)

	group.Add(1)

	go func() {
		defer group.Done()

		for range 10000 {
			run.SetHook(
				thread,
				hook,
			)
			run.SetHook(
				thread,
				nil,
			)
		}
	}()

	for range 10000 {
		run.Poll(thread)
	}

	group.Wait()
}

// TestGuard_C112_HandleFieldsAgainstStore proves a write to the name, the
// thread or the channel of a handle does not race a read of it. Run with -race.
//
// Cases from the Go-level breaker, second pass, findings scheduler-F1 to F9
// (Q207). The breaker wrote the fields with a bare assignment from a second
// goroutine; lark writes them only while it builds a spawn, before any reader
// has the handle. A bare assignment cannot be ordered by a lock, so the fields
// are guarded by the mutex and this test writes through the _Set methods, where
// the breaker assigned.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C112_HandleFieldsAgainstStore(t *testing.T) {
	tests := []struct {
		name  string
		store func(handle *run.Handle)
		read  func(handle *run.Handle)
	}{
		{
			name: "name against String",
			store: func(handle *run.Handle) {
				handle.SetName("updated")
			},
			read: func(handle *run.Handle) {
				_ = handle.String()
			},
		},
		{
			name: "name against Name",
			store: func(handle *run.Handle) {
				handle.SetName("updated")
			},
			read: func(handle *run.Handle) {
				_ = handle.Name()
			},
		},
		{
			name: "thread against String",
			store: func(handle *run.Handle) {
				handle.SetThread("thread_2")
			},
			read: func(handle *run.Handle) {
				_ = handle.String()
			},
		},
		{
			name: "thread against Thread",
			store: func(handle *run.Handle) {
				handle.SetThread("thread_2")
			},
			read: func(handle *run.Handle) {
				_ = handle.Thread()
			},
		},
		{
			name: "done against Done",
			store: func(handle *run.Handle) {
				handle.SetDone(make(chan struct{}))
			},
			read: func(handle *run.Handle) {
				_ = handle.Done()
			},
		},
		{
			name: "done against Result",
			store: func(handle *run.Handle) {
				handle.SetDone(make(chan struct{}))
			},
			read: func(handle *run.Handle) {
				_, _, _ = handle.Result()
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var (
					handle = new(run.Handle)
					group  sync.WaitGroup
				)

				group.Add(1)

				go func() {
					defer group.Done()

					for range 1000 {
						tt.store(handle)
					}
				}()

				for range 1000 {
					tt.read(handle)
				}

				group.Wait()
			},
		)
	}
}

// TestGuard_C112_HandleNilReceiver proves the _Set methods and Chan of a nil
// handle do nothing and do not panic.
//
// Revisions:
//   - 2026-11-08 20:57: initial creation, with _Keep, _Hold, _Work and _Report (C112, B230)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_C112_HandleNilReceiver(t *testing.T) {
	var handle *run.Handle

	handle.SetName("name")
	handle.SetThread("thread")
	handle.SetDone(make(chan struct{}))

	if handle.Chan() != nil {
		t.Fatal("a nil handle has a channel")
	}

	handle.Keep(
		starlark.None,
		nil,
	)
	handle.Hold(func() {
		// Empty
	})

	err := handle.Work(
		new(starlark.Thread),
		&run.Locals{
			Ctx: context.Background(),
		},
		starlark.NewBuiltin(
			"noop",
			func(
				_ *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				return starlark.None, nil
			},
		),
		new(run.Watch),
	)
	if !errors.Is(
		err,
		run.ErrRun,
	) {
		t.Fatalf(
			"_Work of a nil handle is %v, want ErrRun",
			err,
		)
	}

	handle.Report(
		_Started(t.Context()),
		nil,
	)
	new(run.Handle).Report(
		nil,
		nil,
	)
}
