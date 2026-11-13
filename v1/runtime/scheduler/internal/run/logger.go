package run

import (
	"go.starlark.net/starlark"
)

// _Caller is the function that called the builtin running on thread: for a
// print, the function that printed.
//
// The interpreter's own name for it, so a module's top level is <toplevel> and
// an anonymous function is lambda. Empty when no script called it.
//
// Revisions:
//   - 2026-10-02 13:12: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
func _Caller(thread *starlark.Thread) string {
	if thread.CallStackDepth() <= CALLER {
		return ""
	}

	return thread.CallFrame(CALLER).Name
}
