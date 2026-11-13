package tests

// This file tests the Go-level breaker findings flow-F1, F3, F4, F6 and F10 to
// F14 (C100b): WaitBound, PostArgs, NotUpdating, Events.Named and the methods
// of Latch against a nil builtin, a nil receiver or a zero struct. A script
// cannot pass these inputs, so the tests call the functions directly.
//
// Revisions:
//   - 2026-11-06 06:27: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from guard_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/event"
	"github.com/thebagchi/lark/v1/runtime/plugin/event/internal/latch"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestGuard_NilBuiltinHasAName proves a nil builtin names itself ANONYMOUS in
// place of panicking (B129, B131, B132).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-13 01:24: calls NotUpdating from plugin/internal/common (C131, Q417)
func TestGuard_NilBuiltinHasAName(t *testing.T) {
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "wait bound refuses missing arguments",
			call: func() error {
				_, _, err := latch.WaitBound(
					nil,
					nil,
					nil,
				)

				return err
			},
			want: "builtin",
		},
		{
			name: "wait bound reads a duration that is not one",
			call: func() error {
				_, _, err := latch.WaitBound(
					nil,
					starlark.Tuple{starlark.String("e"), starlark.String("x")},
					nil,
				)

				return err
			},
			want: "builtin",
		},
		{
			name: "post args refuses missing arguments",
			call: func() error {
				_, _, err := latch.PostArgs(
					nil,
					starlark.Tuple{},
					nil,
				)

				return err
			},
			want: "builtin",
		},
		{
			name: "not updating names the builtin when the run is missing",
			call: func() error {
				return shared.NotUpdating(
					&starlark.Thread{
						Name: "t",
					},
					nil,
				)
			},
			want: "builtin",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := test.call()
				if err == nil {
					t.Fatal("want an error, got nil")
				}

				if !strings.Contains(
					err.Error(),
					test.want,
				) {
					t.Fatalf(
						"error %q does not name %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}

// TestGuard_ZeroEventsMakesItsMap proves a zero Events answers a name, and
// answers the same latch for the same name (B134).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_ZeroEventsMakesItsMap(t *testing.T) {
	var (
		events = &latch.Events{}
		first  = events.Named("test")
	)

	if first == nil {
		t.Fatal("want a latch, got nil")
	}

	if events.Named("test") != first {
		t.Fatal("a second mention made a second latch")
	}
}

// TestGuard_NilReceivers proves a nil Events and a nil Latch refuse with
// scheduler.ErrRun, and a nil latch has no value (B135, B136, B138).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_NilReceivers(t *testing.T) {
	var (
		events *latch.Events
		latch  *latch.Latch
	)

	if events.Named("test") != nil {
		t.Fatal("a nil _Events answered a latch")
	}

	if err := events.Post(
		"test",
		starlark.None,
	); !errors.Is(
		err,
		scheduler.ErrRun,
	) {
		t.Fatalf(
			"_Events._Post on nil: got %v, want ErrRun",
			err,
		)
	}

	if err := latch.Post(starlark.None); !errors.Is(
		err,
		scheduler.ErrRun,
	) {
		t.Fatalf(
			"_Post on nil: got %v, want ErrRun",
			err,
		)
	}

	_, _, err := latch.Await(
		context.Background(),
		time.Millisecond,
	)
	if !errors.Is(
		err,
		scheduler.ErrRun,
	) {
		t.Fatalf(
			"_Await on nil: got %v, want ErrRun",
			err,
		)
	}

	if latch.Value() != starlark.None {
		t.Fatal("a nil latch has a value")
	}
}

// TestGuard_ZeroLatchPosts proves a zero Latch posts, releases a waiter, and
// refuses a second post (B136).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_ZeroLatchPosts(t *testing.T) {
	latch := new(latch.Latch)

	if err := latch.Post(starlark.MakeInt(7)); err != nil {
		t.Fatalf(
			"first post: %v",
			err,
		)
	}

	value, expired, err := latch.Await(
		context.Background(),
		time.Millisecond,
	)
	if err != nil || expired || value != starlark.MakeInt(7) {
		t.Fatalf(
			"await: got (%v, %v, %v)",
			value,
			expired,
			err,
		)
	}

	if err := latch.Post(starlark.None); !errors.Is(
		err,
		event.ErrPosted,
	) {
		t.Fatalf(
			"second post: got %v, want ErrPosted",
			err,
		)
	}
}

// TestGuard_ZeroLatchWaiterSeesPost proves a waiter that arrives before the
// first post of a zero latch is released by it, not left on a channel nobody
// closes (B136, B137).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_ZeroLatchWaiterSeesPost(t *testing.T) {
	var (
		latch = new(latch.Latch)
		got   = make(
			chan bool,
			1,
		)
	)

	go func() {
		_, expired, _ := latch.Await(
			context.Background(),
			5*time.Second,
		)
		got <- expired
	}()

	time.Sleep(20 * time.Millisecond)

	if err := latch.Post(starlark.None); err != nil {
		t.Fatal(err)
	}

	select {
	case expired := <-got:
		if expired {
			t.Fatal("the waiter timed out")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the waiter was never released")
	}
}

// TestGuard_AwaitAgainstDoneWrites proves _Await holds up when another
// goroutine replaces done under the latch's guard. Run with -race (B137, Q207).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_AwaitAgainstDoneWrites(t *testing.T) {
	var (
		latch = &latch.Latch{
			Done: make(chan struct{}),
		}
		ctx   = context.Background()
		group sync.WaitGroup
	)

	group.Add(2)

	go func() {
		defer group.Done()

		for range 1000 {
			_, _, _ = latch.Await(
				ctx,
				time.Microsecond,
			)
		}
	}()

	go func() {
		defer group.Done()

		for range 1000 {
			latch.Guard.Lock()
			latch.Done = nil
			latch.Guard.Unlock()

			latch.Guard.Lock()
			latch.Done = make(chan struct{})
			latch.Guard.Unlock()
		}
	}()

	group.Wait()
}
