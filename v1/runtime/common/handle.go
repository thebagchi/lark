package common

import (
	"go.starlark.net/starlark"
)

// Handle is one evaluation running beside its caller, as a script sees it. A
// script may hold one, pass it around and join it; it has no attributes and no
// methods of its own, because everything a script can do with one it does
// through join.
//
// It is a starlark.Value, so a script can hold it, and it adds what a host or a
// plugin asks of a running evaluation: when it is over, what it is called, what
// it produced, how to stop it, and which lane it runs on. The scheduler's
// internal package holds the type that implements it (Q127).
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:55: moved from scheduler, as the interface of the Handle struct (C91w5, Q100,
//     Q127)
type Handle interface {
	starlark.Value
	// Done is closed when the evaluation is over.
	Done() <-chan struct{}
	// Name is the function the evaluation runs.
	Name() string
	// Result is what the evaluation produced, whether it is over, and the
	// error it ended with.
	Result() (starlark.Value, bool, error)
	// Stop cancels the evaluation. It does nothing once the evaluation is over.
	Stop()
	// Thread is the id of the lane the evaluation runs on.
	Thread() string
}
