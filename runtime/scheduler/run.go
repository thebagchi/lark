package scheduler

import (
	"context"
	"errors"
	"sync"

	"go.starlark.net/starlark"
)

// ErrNoRun is returned when a builtin is called on a thread no run set up.
var ErrNoRun = errors.New("no run on this thread")

const (
	// RUN_KEY and THREAD_KEY name what a run leaves on an interpreter thread.
	// A Starlark builtin is handed only a thread, so this is how spawn, join
	// and cancel find the run they belong to.
	RUN_KEY    = "scheduler.run"
	THREAD_KEY = "scheduler.thread"

	// SPINE is the entry point's thread number and FIRST_SPAWN is the first a
	// spawn takes. workflow.proto records both as int32.
	SPINE       = 0
	FIRST_SPAWN = 1
)

// _Run is one entry point's execution: every handle it started, so the ones
// nobody joined can be cancelled when it returns.
//
// The context lives here rather than being passed, because a Starlark builtin
// is handed only a thread and a builtin is where spawn runs. go.md forbids a
// context on a struct that outlives the call it was scoped to; a _Run is
// created by a call into an artifact and discarded when that call returns, so
// it does not.
type _Run struct {
	ctx   context.Context
	group sync.WaitGroup
	mutex sync.Mutex
	live  []*Handle
	next  int32
}

// _Abandon cancels every handle still running.
//
// A handle nobody joined is work the entry point did not wait for, and the run
// is over when the entry point says so. Cancelling rather than waiting is what
// stops a forgotten spawn from holding a run open.
//
// Revisions:
//   - 2026-09-19 20:35: initial creation
func (r *_Run) _Abandon() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for _, handle := range r.live {
		handle.stop()
	}
}

// _Track records a handle as running and gives it its thread number.
//
// Numbering happens under the same lock that records the handle, so the number
// a handle carries is the position it was started in and two spawns racing
// cannot be given one number.
//
// Revisions:
//   - 2026-09-19 20:36: initial creation
//   - 2026-09-19 20:51: assigns the thread number, so a schema that records one
//     reads it rather than reconstructing it
func (r *_Run) _Track(handle *Handle) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	handle.thread = r.next
	r.next++

	r.live = append(r.live, handle)
}

// _Of returns the run a thread belongs to.
//
// Revisions:
//   - 2026-09-19 20:36: initial creation
func _Of(thread *starlark.Thread) (*_Run, error) {
	run, ok := thread.Local(RUN_KEY).(*_Run)
	if !ok {
		return nil, ErrNoRun
	}

	return run, nil
}

// Begin attaches a new run to thread and returns the function that ends it.
//
// This is the whole of what another package needs from this one: a call that
// wants spawn, join and cancel to work on its thread calls Begin, defers what
// it returns, and evaluates in between.
//
// Ending a run cancels every handle nobody joined and then waits for every
// goroutine it started, so no spawned thread outlives the call that made it.
// The thread itself is cancelled when ctx is, which is how a caller stops an
// evaluation already running.
//
// It exists because the plan's phase 8 was written while this was one package
// and said Invoke would build a _Run itself. The package split that followed
// made that impossible without exporting the run, its tracking and its
// abandoning - three things a caller has no business touching. One function
// that brackets a call is the smaller surface.
//
// Revisions:
//   - 2026-09-19 22:39: initial creation
func Begin(ctx context.Context, thread *starlark.Thread) func() {
	run := &_Run{
		ctx:  ctx,
		next: FIRST_SPAWN,
	}

	thread.SetLocal(RUN_KEY, run)
	thread.SetLocal(THREAD_KEY, int32(SPINE))

	stop := _CancelOn(ctx, thread)

	return func() {
		stop()
		run._Abandon()
		run.group.Wait()
	}
}
