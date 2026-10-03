package scheduler

import (
	"go.starlark.net/starlark"
)

const (
	// CALLER is how deep a builtin's caller sits on the interpreter's call
	// stack: depth 0 is the builtin itself.
	CALLER = 1

	// ATTR_THREAD and ATTR_FUNCTION are the attributes a printed line is logged
	// with: the lane that printed it, and the function that did.
	ATTR_THREAD   = "thread"
	ATTR_FUNCTION = "function"
)

// _Caller is the function that called the builtin running on thread: for a
// print, the function that printed.
//
// The interpreter's own name for it, so a module's top level is <toplevel> and
// an anonymous function is lambda. Empty when no script called it.
//
// Revisions:
//   - 2026-10-02 13:12: initial creation
func _Caller(thread *starlark.Thread) string {
	if thread.CallStackDepth() <= CALLER {
		return ""
	}

	return thread.CallFrame(CALLER).Name
}
