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

// Handle is one spawned function, as a script sees it. A script may hold one,
// pass it around and join it; it has no attributes and no methods, because
// everything a script can do with one it does through join.
type Handle struct {
	name   string
	thread int32
	done   chan struct{}
	stop   context.CancelFunc
	value  starlark.Value
	err    error
}

// String names the function this handle is running.
//
// Revisions:
//   - 2026-09-19 20:26: initial creation
func (h *Handle) String() string {
	return fmt.Sprintf("<%s %s #%d>", HANDLE_TYPE, h.name, h.thread)
}

// Thread is the number this handle's evaluation runs under.
//
// Numbers are what a workflow schema records - a thread's index, the thread a
// fork starts, the threads a join waits for - so they are assigned here, where
// a thread is actually started, rather than reconstructed afterwards by
// something that would have to guess the order.
//
// The entry point is thread 0 and is not a handle; spawns take 1 upwards in the
// order the script made them, which is the order a re-run of the same script
// makes them again.
//
// Revisions:
//   - 2026-09-19 20:50: initial creation
func (h *Handle) Thread() int32 {
	return h.thread
}

// Name is the function this handle is running.
//
// Revisions:
//   - 2026-09-19 20:50: initial creation
func (h *Handle) Name() string {
	return h.name
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
