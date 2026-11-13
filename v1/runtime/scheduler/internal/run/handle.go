package run

import (
	"context"
	"fmt"
	"sync"

	"go.starlark.net/starlark"
)

// Handle is one evaluation running beside its caller, as a script sees it. A
// script may hold one, pass it around and join it; it has no attributes and no
// methods, because everything a script can do with one it does through join.
//
// A finished handle lets go of its stop, which is how a handle a script keeps
// holds a result and not a run. mutex guards stop against that, and guards name,
// thread and done against a write beside a read (Q207): every read goes through
// a method and every write through a _Set method, which take it.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 19:23: mutex also guards name, thread and done, read by the methods and
//     written by the _Set methods (C112, Q207)
//   - 2026-11-10 00:25: renamed the fields stop, value, as a test in tests/ needs them
//     (C120a, Q314)
type Handle struct {
	mutex  sync.Mutex
	name   string
	thread string
	done   chan struct{}
	Cancel context.CancelFunc
	Value  starlark.Value
	err    error
}

// String names the function this handle is running.
//
// Revisions:
//   - 2026-09-19 20:26: initial creation
//   - 2026-09-21 00:59: a thread id is a string
//   - 2026-11-04 23:50: a nil Handle names nothing (C86 fix round 2)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:08: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-08 19:23: reads the name and the thread under the mutex (C112, Q207)
func (h *Handle) String() string {
	if h == nil {
		h = new(Handle)
	}

	return fmt.Sprintf(
		"<%s %s #%s>",
		HANDLE_TYPE,
		h.Name(),
		h.Thread(),
	)
}

// Thread is the id of the lane this handle's evaluation runs on.
//
// Ids are what a workflow schema records - a thread's own id, the thread a
// spawn starts, the threads a join waits for - so they are assigned where a
// thread is actually started, rather than reconstructed afterwards by
// something that would have to guess the order. The entry point is thread_0
// and is not a handle; a spawn's id names the thread that spawned it.
//
// Revisions:
//   - 2026-09-19 20:50: initial creation
//   - 2026-09-21 08:09: an id names its parent
//   - 2026-11-04 23:50: a nil Handle has no id (C86 fix round 2)
//   - 2026-11-08 19:23: reads the thread under the mutex (C112, Q207)
func (h *Handle) Thread() string {
	if h == nil {
		return ""
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.thread
}

// Name is the function this handle is running.
//
// Revisions:
//   - 2026-09-19 20:50: initial creation
//   - 2026-11-04 23:50: a nil Handle has no name (C86 fix round 2)
//   - 2026-11-08 19:23: reads the name under the mutex (C112, Q207)
func (h *Handle) Name() string {
	if h == nil {
		return ""
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.name
}

// Done is closed when the evaluation is over, whichever way it ended.
//
// For a caller that waits on more than one thing at once - a timeout against
// its clock. A caller waiting on the handle alone uses Wait, which also
// watches the caller's own cancellation.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-04 23:50: a nil or zero Handle is over, so its Done is closed (C86 fix round 2)
//   - 2026-11-08 19:23: reads the channel under the mutex (C112, Q207)
//   - 2026-11-10 01:00: now uses Chan, were _Chan (C120a, Q314)
func (h *Handle) Done() <-chan struct{} {
	if h != nil {
		done := h.Chan()
		if done != nil {
			return done
		}
	}

	over := make(chan struct{})
	close(over)

	return over
}

// Result is what the evaluation produced, and whether it is over. It reads the
// handle alone: unlike Wait it never answers with the caller's cancellation,
// so a record of a finished handle comes from the handle and not from the
// caller's context. While the evaluation runs it answers nil, false and nil.
//
// The result is read under the mutex that Keep stores it under, so a read and a
// store never race, whatever order they come in.
//
// Revisions:
//   - 2026-11-02 16:44: initial creation (C28)
//   - 2026-11-04 00:30: reads the result under the mutex (C54, B079)
//   - 2026-11-04 23:50: a nil or zero Handle is over with no value and no error (C86 fix round 2)
//   - 2026-11-08 19:23: reads the channel and the result under one hold of the mutex (C112, Q207)
//   - 2026-11-10 01:00: now uses Keep, Value, were _Keep, value (C120a, Q314)
func (h *Handle) Result() (starlark.Value, bool, error) {
	if h == nil {
		return nil, true, nil
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.done == nil {
		return nil, true, nil
	}

	select {
	case <-h.done:
		return h.Value, true, h.err

	default:
		return nil, false, nil
	}
}

// Stop cancels the evaluation without waiting for it. It does nothing once the
// evaluation is over.
//
// A nil handle has nothing to stop.
//
// The stop function must not call Stop on the same handle: Stop calls it with the
// handle's stop still in place, so the two would call each other without end
// (Q274). The stop lark sets is a context.CancelFunc, which never does.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-02 12:05: reads the stop under the mutex, since a finished
//     handle lets it go
//   - 2026-11-04 00:30: a nil handle does nothing (C54, B090)
//   - 2026-11-04 23:50: a nil Handle has nothing to stop (C86 fix round 2)
//   - 2026-11-08 20:57: the stop function must not call Stop (C112 fix round 1, Q274)
//   - 2026-11-10 01:00: now uses Cancel, were stop (C120a, Q314)
func (h *Handle) Stop() {
	if h == nil {
		return
	}

	h.mutex.Lock()
	stop := h.Cancel
	h.mutex.Unlock()

	if stop != nil {
		stop()
	}
}

// Type names this value's type, as a script's type() sees it.
//
// Revisions:
//   - 2026-09-19 20:26: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:08: moved to package run; its names are used unqualified (C91w5, Q116)
func (h *Handle) Type() string {
	return HANDLE_TYPE
}

// Freeze is empty: a handle carries nothing a script can mutate, so there is
// nothing for freezing to make safe.
//
// Revisions:
//   - 2026-09-19 20:26: initial creation
func (h *Handle) Freeze() {
	// Empty
}

// Truth reports a handle as true, so a script can test one for presence.
//
// Revisions:
//   - 2026-09-19 20:26: initial creation
func (h *Handle) Truth() starlark.Bool {
	return starlark.True
}

// Hash refuses, because a handle is identity rather than value and two runs of
// one function are two different handles.
//
// Revisions:
//   - 2026-09-19 20:26: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:08: moved to package run; its names are used unqualified (C91w5, Q116)
func (h *Handle) Hash() (uint32, error) {
	return 0, fmt.Errorf(
		"%s is unhashable",
		HANDLE_TYPE,
	)
}

// Detach takes the stop function out of the handle and returns it, so a
// finished handle keeps no reference to the context. A stop after that does
// nothing, which is what a stop of a finished evaluation always did.
//
// Returns a function that does nothing when the handle has none.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation, for the Q13 decision of change C33
//   - 2026-11-03 21:57: a nil handle has nothing to stop, so it gives the function that does
//     nothing (C52, B073)
//   - 2026-11-10 00:25: renamed from _Detach, as a test in tests/ needs it (C120a, Q314)
func (h *Handle) Detach() context.CancelFunc {
	if h == nil {
		return func() {
			// Empty
		}
	}

	h.mutex.Lock()
	stop := h.Cancel
	h.Cancel = nil
	h.mutex.Unlock()

	if stop == nil {
		return func() {
			// Empty
		}
	}

	return stop
}

// Keep stores what the evaluation produced, under the mutex Result reads it
// under.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation, so Result and the store never race (C54, B079)
//   - 2026-11-08 21:04: a nil receiver stores nothing (C112 fix round 2, B230, Q65)
//   - 2026-11-10 00:25: renamed from _Keep, as a test in tests/ needs it (C120a, Q314)
func (h *Handle) Keep(
	value starlark.Value,
	err error,
) {
	if h == nil {
		return
	}

	h.mutex.Lock()
	h.Value = value
	h.err = err
	h.mutex.Unlock()
}

// Hold sets the stop function under the mutex Stop and Detach read and clear
// it under, so every write to stop is locked.
//
// The stop function must not call Stop on this handle (Q274).
//
// Revisions:
//   - 2026-11-04 00:30: initial creation, so no write to stop is bare (C54 fix round 1, B091)
//   - 2026-11-08 20:57: the stop function must not call Stop; a nil receiver holds nothing
//     (C112, Q274, B230)
//   - 2026-11-10 00:25: renamed from _Hold, as a test in tests/ needs it (C120a, Q314)
func (h *Handle) Hold(stop context.CancelFunc) {
	if h == nil {
		return
	}

	h.mutex.Lock()
	h.Cancel = stop
	h.mutex.Unlock()
}

// SetName sets the name of the function this handle runs, under the mutex Name
// reads it under.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-10 00:25: renamed from _SetName, as a test in tests/ needs it (C120a, Q314)
func (h *Handle) SetName(name string) {
	if h == nil {
		return
	}

	h.mutex.Lock()
	h.name = name
	h.mutex.Unlock()
}

// SetThread sets the id of the lane this handle runs on, under the mutex
// Thread reads it under.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-10 00:25: renamed from _SetThread, as a test in tests/ needs it (C120a, Q314)
func (h *Handle) SetThread(thread string) {
	if h == nil {
		return
	}

	h.mutex.Lock()
	h.thread = thread
	h.mutex.Unlock()
}

// SetDone sets the channel that closes when the evaluation is over, under the
// mutex Done and Result read it under.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-10 00:25: renamed from _SetDone, as a test in tests/ needs it (C120a, Q314)
func (h *Handle) SetDone(done chan struct{}) {
	if h == nil {
		return
	}

	h.mutex.Lock()
	h.done = done
	h.mutex.Unlock()
}

// Chan is the channel SetDone set, or nil when none was. Unlike Done it makes
// no closed channel for a handle with none, so the one that ends the evaluation
// can close the real one.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-10 00:25: renamed from _Chan, as a test in tests/ needs it (C120a, Q314)
func (h *Handle) Chan() chan struct{} {
	if h == nil {
		return nil
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.done
}
