// Package scheduler runs a Starlark script's concurrent work: a spawned
// function gets a goroutine and an interpreter thread of its own, and a handle
// is what a script holds it by.
package scheduler

import (
	"context"
	"fmt"

	"go.starlark.net/starlark"
)

// HANDLE_TYPE is what a script's type() reports for a handle.
const HANDLE_TYPE = "handle"

// Handle is one evaluation running beside its caller, as a script sees it. A
// script may hold one, pass it around and join it; it has no attributes and no
// methods, because everything a script can do with one it does through join.
//
// raw is the error the evaluation actually produced, kept beside the one a
// joiner is handed, so a report can tell the thread that ended a run from the
// threads that ending stopped.
type Handle struct {
	name   string
	thread string
	done   chan struct{}
	stop   context.CancelFunc
	value  starlark.Value
	err    error
	raw    error
}

// String names the function this handle is running.
//
// Revisions:
//   - 2026-09-19 20:26: initial creation
//   - 2026-09-21 00:59: a thread id is a string
func (h *Handle) String() string {
	return fmt.Sprintf("<%s %s #%s>", HANDLE_TYPE, h.name, h.thread)
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
func (h *Handle) Thread() string {
	return h.thread
}

// Name is the function this handle is running.
//
// Revisions:
//   - 2026-09-19 20:50: initial creation
func (h *Handle) Name() string {
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
func (h *Handle) Done() <-chan struct{} {
	return h.done
}

// Stop cancels the evaluation without waiting for it.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func (h *Handle) Stop() {
	h.stop()
}

// Type names this value's type, as a script's type() sees it.
//
// Revisions:
//   - 2026-09-19 20:26: initial creation
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
func (h *Handle) Hash() (uint32, error) {
	return 0, fmt.Errorf("%s is unhashable", HANDLE_TYPE)
}
