package run

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// Lane is the id of the lane the evaluation on thread runs on.
//
// The spine when a thread carries none, which is a thread nothing set up. An id
// is only ever used to group what is reported, so a wrong lane is a tidier
// failure than no answer at all.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-21 00:59: a thread id is a string that names its parent
//   - 2026-09-21 08:09: read from the one local
//   - 2026-10-03 08:26: unexported as _Lane, nothing outside this package asking it
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: now uses Of, ThreadID, were _Of, thread (C120a, Q314)
//   - 2026-11-10 11:55: renamed from _Lane, as scheduler.Lane forwards to it (C116, B237, Q295)
func Lane(thread *starlark.Thread) string {
	locals, err := Of(thread)
	if err != nil {
		return common.SPINE
	}

	return locals.ThreadID
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
//   - 2026-11-10 01:00: now uses Of, Run, ThreadID, were _Of, run, thread (C120a, Q314)
func Open(
	thread *starlark.Thread,
	line *common.Line,
) {
	locals, err := Of(thread)
	if err != nil {
		return
	}

	locals.Run._Tell(func() {
		locals.Run.into.Started(
			locals.ThreadID,
			line,
		)
	})
}

// Close tells whatever is watching the run on thread how the line that called
// name ended, through the same guard as Open.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation, from flow's _Finished, so every
//     builtin reports a line one way
//   - 2026-11-03 21:57: a nil thread carries no run, so nothing is told (C52, B058)
//   - 2026-11-10 01:00: now uses Of, Run, ThreadID, were _Of, run, thread (C120a, Q314)
func Close(
	thread *starlark.Thread,
	name string,
	err error,
) {
	locals, failure := Of(thread)
	if failure != nil {
		return
	}

	locals.Run._Tell(func() {
		locals.Run.into.Ended(
			locals.ThreadID,
			name,
			err,
		)
	})
}
