package scheduler

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// Lane returns the id of the lane the evaluation on thread runs on, which is
// the id Handle.Thread gives for a thread that owns its lane. It is the spine
// for a thread nothing set up.
//
// For a builtin that must tell the calling thread from another: join refuses
// the handle whose lane is the caller's.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation; proposed name (C116, B237, Q295)
func Lane(thread *starlark.Thread) string {
	return run.Lane(thread)
}

// Open tells whatever is watching the run on thread that a line has started,
// on the lane thread runs on.
//
// Through the run's guard, so a reporter that raises ends the run as one that
// raises on a spawn does, rather than failing whatever builtin reported the
// line. Nothing to tell when nothing is listening, or when thread carries no
// run.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation, from flow's _Began, so every builtin
//     reports a line one way
//   - 2026-11-03 21:57: a nil thread carries no run, so nothing is told (C52, B057)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:10: forwards to run.Open (C91w5)
func Open(
	thread *starlark.Thread,
	line *common.Line,
) {
	run.Open(
		thread,
		line,
	)
}

// Close tells whatever is watching the run on thread how the line that called
// name ended, through the same guard as Open.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation, from flow's _Finished, so every
//     builtin reports a line one way
//   - 2026-11-03 21:57: a nil thread carries no run, so nothing is told (C52, B058)
//   - 2026-11-07 20:10: forwards to run.Close (C91w5)
func Close(
	thread *starlark.Thread,
	name string,
	err error,
) {
	run.Close(
		thread,
		name,
		err,
	)
}
