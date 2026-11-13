// This file tests the guards of the run package against inputs no exported call
// builds (the breaker guard tests of C100a, Q206, Q207). The inputs are:
//   - ForkNil: ForkHandle with a nil parent, or a parent whose run is nil;
//     ForkGo with a nil spawn or flight; Inherited with a nil parent;
//     Inherit with a nil child. Fork reads its parent through Of, which
//     never returns nil locals or locals with no run.
//   - WorkNil: Work with a nil thread, nil locals or a nil watch. Fork
//     passes real ones.
//   - WatchNil: Look on a nil watch, a nil thread or a nil status; Release
//     on a nil or an unarmed watch; Arm on a nil watch or a nil thread; and
//     Arm on one watch from 100 goroutines at once. Begin and Fork each make
//     their own watch.
//   - ReleaseNil: Release on a nil *Locals (Release's type assertion cannot
//     give a typed nil), and on a hold set with no slot (Lock sets the slot
//     before the hold).
//   - RunNil: Ended with a nil done (Begin always passes one), Outcome on a
//     nil run, Fail on locals with no run or on nil locals (Of refuses
//     those first), and Logging with nil settings (Begin swaps in empty
//     settings first).
//
// The logger race and the release of token 0 are in logger_test.go and
// release_test.go.
//
// Revisions:
//   - 2026-11-06 06:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:45: the header names the Q206 and Q207 decisions and the
//     exact input each test needs (C65c)
//   - 2026-11-10 00:25: moved to tests/ from guard_c100a_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
//   - 2026-11-12 19:18: header names Fork, ForkHandle, ForkGo and ForkNil,
//     formerly the Beside names (C129, Q396)
package tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// TestGuard_C100a_ForkNil proves the spawn helpers refuse or skip nil input.
//
// Cases from Go-level breaker, findings scheduler-F53 to F57 (bugs B156 to
// B160). ForkHandle gives ErrRun; the others do nothing or read a nil
// parent as one with no run.
//
// Revisions:
//   - 2026-11-06 00:02: initial creation (C100a)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
//   - 2026-11-12 18:58: renamed from TestGuard_C100a_BesideNil (C129, Q396)
func TestGuard_C100a_ForkNil(t *testing.T) {
	t.Run(
		"ForkHandle nil parent",
		func(t *testing.T) {
			_, _, err := run.ForkHandle(
				nil,
				nil,
				nil,
				nil,
			)
			if !errors.Is(
				err,
				run.ErrRun,
			) {
				t.Fatalf(
					"got %v, want ErrRun",
					err,
				)
			}
		},
	)

	t.Run(
		"ForkHandle parent with no run",
		func(t *testing.T) {
			_, _, err := run.ForkHandle(
				nil,
				&run.Locals{
					Ctx: context.Background(),
				},
				nil,
				nil,
			)
			if !errors.Is(
				err,
				run.ErrRun,
			) {
				t.Fatalf(
					"got %v, want ErrRun",
					err,
				)
			}
		},
	)

	t.Run(
		"ForkGo nil spawn and flight",
		func(t *testing.T) {
			run.ForkGo(
				nil,
				nil,
			)
			run.ForkGo(
				new(run.Spawn),
				nil,
			)
			run.ForkGo(
				new(run.Spawn),
				new(run.Flight),
			)
		},
	)

	t.Run(
		"Inherited nil parent",
		func(t *testing.T) {
			child := run.Inherited(
				nil,
				[]common.ForkOption{run.Catching()},
			)
			if child == nil || child.Run != nil || child.Catching != 1 {
				t.Fatalf(
					"got %+v, want locals with no run and catching 1",
					child,
				)
			}
		},
	)

	t.Run(
		"Inherit nil child",
		func(t *testing.T) {
			run.Inherit(
				&run.Locals{},
				nil,
				nil,
			)
		},
	)
}

// TestGuard_C100a_WorkNil proves Work refuses a missing thread, locals or
// watch with ErrRun.
//
// Cases from Go-level breaker, findings scheduler-F59 and F60 (bugs B161 and
// B162).
//
// Revisions:
//   - 2026-11-06 00:02: initial creation (C100a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_C100a_WorkNil(t *testing.T) {
	var (
		thread = new(starlark.Thread)
		locals = &run.Locals{
			Ctx: context.Background(),
		}
		watch = new(run.Watch)
	)

	tests := []struct {
		name    string
		thread  *starlark.Thread
		locals  *run.Locals
		watcher *run.Watch
	}{
		{
			"nil watch",
			thread,
			locals,
			nil,
		},
		{
			"nil locals",
			thread,
			nil,
			watch,
		},
		{
			"nil thread",
			nil,
			locals,
			watch,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				err := new(run.Handle).Work(
					tt.thread,
					tt.locals,
					nil,
					tt.watcher,
				)
				if !errors.Is(
					err,
					run.ErrRun,
				) {
					t.Fatalf(
						"got %v, want ErrRun",
						err,
					)
				}
			},
		)
	}
}

// TestGuard_C100a_WatchNil proves Look and Release take nil input, and Arm
// takes many callers at once.
//
// Cases from Go-level breaker, findings scheduler-F61 and F62 (bugs B163 and
// B164; Q207 for the race). Run it with -race: before the guard, 100 callers
// wrote the watch's fields together.
//
// Revisions:
//   - 2026-11-06 00:02: initial creation (C100a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C100a_WatchNil(t *testing.T) {
	t.Run(
		"Look nil watch",
		func(t *testing.T) {
			var watch *run.Watch

			watch.Look(new(starlark.Thread))
		},
	)

	t.Run(
		"Look nil thread",
		func(t *testing.T) {
			watch := &run.Watch{
				Status: func() error {
					return nil
				},
			}

			watch.Look(nil)
		},
	)

	t.Run(
		"Look nil status",
		func(t *testing.T) {
			new(run.Watch).Look(new(starlark.Thread))
		},
	)

	t.Run(
		"Release never armed",
		func(t *testing.T) {
			new(run.Watch).Release()

			var watch *run.Watch

			watch.Release()
		},
	)

	t.Run(
		"Arm nil",
		func(t *testing.T) {
			var watch *run.Watch

			watch.Arm(
				context.Background(),
				new(starlark.Thread),
			)
			new(run.Watch).Arm(
				context.Background(),
				nil,
			)
		},
	)

	t.Run(
		"Arm together",
		func(t *testing.T) {
			var (
				watch  = new(run.Watch)
				thread = new(starlark.Thread)
				group  sync.WaitGroup
			)

			for range 100 {
				group.Add(1)

				go func() {
					defer group.Done()

					watch.Arm(
						context.Background(),
						thread,
					)
				}()
			}

			group.Wait()

			if watch.Thread.Load() != thread {
				t.Fatal("watch is not armed on the thread")
			}
		},
	)
}

// TestGuard_C100a_ReleaseNil proves Release returns for a nil receiver and for
// a hold with no slot, where it blocked for ever. The release of token 0 is
// TestGuard_C100a_ReleaseTokenZero, in release_test.go.
//
// Case from Go-level breaker, finding scheduler-F64 (bug B165).
//
// Revisions:
//   - 2026-11-06 00:02: initial creation (C100a)
//   - 2026-11-09 02:09: the row for token 0 moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C100a_ReleaseNil(t *testing.T) {
	tests := []struct {
		name   string
		locals *run.Locals
		token  uint64
	}{
		{
			"nil receiver",
			nil,
			1,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				ended := make(chan struct{})

				go func() {
					defer close(ended)

					tt.locals.Release(tt.token)
				}()

				select {
				case <-ended:
				case <-time.After(2 * time.Second):
					t.Fatal("_Release did not return")
				}
			},
		)
	}

	t.Run(
		"held token with no slot",
		func(t *testing.T) {
			locals := new(run.Locals)
			locals.HoldNumber.Store(7)

			ended := make(chan struct{})

			go func() {
				defer close(ended)

				locals.Release(7)
			}()

			select {
			case <-ended:
			case <-time.After(2 * time.Second):
				t.Fatal("_Release did not return")
			}
		},
	)
}

// TestGuard_C100a_RunNil proves Ended, Outcome, Fail and Logging take nil
// input.
//
// Cases from Go-level breaker, findings scheduler-F66, F68, F69 and F70 (bugs
// B167, B168, B169 and B170).
//
// Revisions:
//   - 2026-11-06 00:02: initial creation (C100a)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/, with the local run renamed (C120a, Q311, Q321)
func TestGuard_C100a_RunNil(t *testing.T) {
	cause := errors.New("cause")

	t.Run(
		"Ended nil done",
		func(t *testing.T) {
			err := run.Ended(
				context.Background(),
				nil,
			)(cause)
			if !errors.Is(
				err,
				cause,
			) {
				t.Fatalf(
					"got %v, want the cause",
					err,
				)
			}
		},
	)

	t.Run(
		"Outcome nil run",
		func(t *testing.T) {
			var nilRun *run.Run

			if err := nilRun.Outcome(); err != nil {
				t.Fatalf(
					"got %v, want nil",
					err,
				)
			}
		},
	)

	t.Run(
		"Fail locals with no run",
		func(t *testing.T) {
			err := new(run.Locals).Fail(cause)
			if !errors.Is(
				err,
				cause,
			) {
				t.Fatalf(
					"got %v, want the cause",
					err,
				)
			}

			var locals *run.Locals

			err = locals.Fail(cause)
			if !errors.Is(
				err,
				cause,
			) {
				t.Fatalf(
					"nil locals: got %v, want the cause",
					err,
				)
			}
		},
	)

	t.Run(
		"Logging nil settings",
		func(t *testing.T) {
			var settings *common.Settings

			if run.Logging(settings) == nil {
				t.Fatal("got nil, want the default logger")
			}
		},
	)
}
