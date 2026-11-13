package scheduler

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// Locate returns err with the position of the innermost Starlark frame of
// thread in front of its text, for a builtin to raise.
//
// Only for a builtin running on the interpreter goroutine of thread, where the
// stack is its own to read. Fail cannot do this itself: the file plugin's
// walker calls Fail with the thread it was made for, from a goroutine that may
// not be the one running it, and reading that thread's stack is a data race.
//
// Revisions:
//   - 2026-11-04 07:10: initial creation (C69)
//   - 2026-11-07 20:10: forwards to run.Locate (C91w5)
func Locate(
	thread *starlark.Thread,
	err error,
) error {
	return run.Locate(
		thread,
		err,
	)
}
