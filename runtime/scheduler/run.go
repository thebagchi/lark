package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.starlark.net/starlark"
)

var (
	// ErrNoRun is returned when a builtin is called on a thread no run set up.
	ErrNoRun = errors.New("no run on this thread")

	// ErrNotLocal is returned when two plugins claim one key for values of
	// different types.
	ErrNotLocal = errors.New("a run local holds another type")
)

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
	ctx     context.Context
	stop    context.CancelFunc
	group   sync.WaitGroup
	mutex   sync.Mutex
	live    []*Handle
	next    int32
	outcome error
	locals  map[string]any
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
	inner, stop := context.WithCancel(ctx)

	run := &_Run{
		ctx:  inner,
		stop: stop,
		next: FIRST_SPAWN,
	}

	thread.SetLocal(RUN_KEY, run)
	thread.SetLocal(THREAD_KEY, int32(SPINE))
	thread.SetLocal(CONTEXT_KEY, inner)

	watching := _CancelOn(inner, thread)

	return func() {
		watching()
		run._Abandon()
		run.group.Wait()
		stop()
	}
}

// _End records the run's outcome and stops everything it started.
//
// A run has one outcome, and it is whatever was recorded first. Everything that
// happens afterwards - a spinning sibling cancelled, a thread told to stop
// between instructions, an evaluation unwinding - is the shutdown, not the
// reason for it.
//
// Separating the two is what removes the need to reconstruct a reason from the
// errors cancellation produces. Nothing has to decide whether an error means
// "this failed" or "this was stopped because something else failed": the
// outcome already says which failure mattered, and the rest may report
// cancellation freely.
//
// Revisions:
//   - 2026-09-20 00:07: initial creation, as _Fail
//   - 2026-09-20 00:12: named for what it records rather than what it does, and
//     documented as the run's one authoritative answer
func (r *_Run) _End(outcome error) {
	r.mutex.Lock()

	if r.outcome == nil {
		r.outcome = outcome
	}

	r.mutex.Unlock()

	r.stop()
}

// _Outcome returns what ended this run, or nil if nothing did.
//
// Revisions:
//   - 2026-09-20 00:12: initial creation
func (r *_Run) _Outcome() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.outcome
}

// Outcome returns what ended the run on thread, or nil if nothing did.
//
// A caller asks this **after** ending the run, never before: ending waits for
// every thread the run started, and a thread still running is a thread that can
// still fail. Asked too early, this reports nothing and a run that had already
// been stopped looks like a success.
//
// Revisions:
//   - 2026-09-20 00:08: initial creation, as Cause
//   - 2026-09-20 00:12: renamed, and documented as something to ask last
func Outcome(thread *starlark.Thread) error {
	run, err := _Of(thread)
	if err != nil {
		return nil
	}

	return run._Outcome()
}

// Local returns this run's value for key, building it the first time it is
// asked for.
//
// It is how a plugin holds something that belongs to one execution rather than
// to the process or to a compile - a store two spawned threads share, say. Two
// runs of one artifact get two values; the threads within a run get the same
// one, because they carry the same run.
//
// build is called at most once per run per key, under the lock, so two threads
// racing to be first still see one value.
//
// The map is keyed to any because what a plugin stores is its own business and
// no two plugins store the same shape. The type parameter is what keeps that
// contained: a caller names the type it expects and never sees the assertion.
//
// Returns ErrNoRun when the thread has no run, and ErrNotLocal when key already
// holds something of another type - which means two plugins chose one key.
//
// Revisions:
//   - 2026-09-20 00:31: initial creation
func Local[T any](thread *starlark.Thread, key string, build func() T) (T, error) {
	var empty T

	run, err := _Of(thread)
	if err != nil {
		return empty, err
	}

	run.mutex.Lock()
	defer run.mutex.Unlock()

	if run.locals == nil {
		run.locals = map[string]any{}
	}

	held, found := run.locals[key]
	if !found {
		made := build()
		run.locals[key] = made

		return made, nil
	}

	value, ok := held.(T)
	if !ok {
		return empty, fmt.Errorf("%s holds a %T: %w", key, held, ErrNotLocal)
	}

	return value, nil
}

// End records outcome as why the run on thread ended, and stops it.
//
// It is how something that catches assertions gives up: retry catches each
// attempt so that one failure is not the end of everything, and then has to end
// the run itself when no attempt succeeded.
//
// Revisions:
//   - 2026-09-20 01:20: initial creation
func End(thread *starlark.Thread, outcome error) {
	run, err := _Of(thread)
	if err != nil {
		return
	}

	run._End(outcome)
}
