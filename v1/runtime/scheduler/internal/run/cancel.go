package run

import (
	"context"
	"math"
	"sync/atomic"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// Watch is the watch a thread keeps on its context, held as the context's Err
// method: the thread looks at the context itself, every common.POLL
// interpreter steps, and cancels itself when the context is done.
//
// The watch started a goroutine for the cancel before (context.AfterFunc).
// That goroutine had to be scheduled before it could cancel a thread that was
// itself running, and with more threads than processors it waited behind them
// for a time slice each: 256 spinning threads took about 2.5 s to stop. A
// thread that looks for itself needs no other goroutine. Source: the research
// cancel_latency.md and cancel_overhead.md.
//
// thread is claimed by one compare and swap, so callers that arm one watch
// together arm it once, and the field costs the same pointer it did before.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: renamed from _Watch and the fields status, thread, as a test in tests/
//     needs them (C120a, Q314)
type Watch struct {
	Status func() error
	Thread atomic.Pointer[starlark.Thread]
}

// Arm makes thread cancel itself once ctx is done, on a watch the caller
// owns, so a spawn can keep the watch in the object that holds its thread. The
// release of the watch stops that.
//
// It arms the interpreter's step limit, which calls the watch's hook. The
// first limit is one step away, so the first instruction looks at ctx: a
// thread started under a context already done stops at once. The hook sets the
// next limit itself, common.POLL steps on.
//
// Both fields it sets belong to the thread, which only the goroutine that
// runs it may touch. The caller runs it before the thread starts.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation, out of _CancelOn, for change C33
//   - 2026-11-04 00:30: sets the hook with _SetHook (C54 fix round 1, B093)
//   - 2026-11-05 19:07: takes the doc of _CancelOn, which only called it, and is gone (C58)
//   - 2026-11-06 00:02: the first of many callers at once arms the watch and the rest return,
//     and a
//     nil thread or watch arms nothing (C100a, B163, Q207)
//   - 2026-11-10 00:25: renamed from _Arm, as a test in tests/ needs it (C120a, Q314)
func (w *Watch) Arm(
	ctx context.Context,
	thread *starlark.Thread,
) {
	if w == nil || thread == nil || !w.Thread.CompareAndSwap(
		nil,
		thread,
	) {
		return
	}

	w.Status = ctx.Err

	SetHook(
		thread,
		w.Look,
	)
	thread.SetMaxExecutionSteps(thread.ExecutionSteps() + 1)
}

// Release stops watching, and returns at once. A second release does the same.
//
// It runs on the goroutine that runs the thread, or after that goroutine has
// finished with it, so it does not race the hook. The thread then runs under
// no step limit, and a context cancelled after the release cannot cancel it.
//
// Revisions:
//   - 2026-10-28 22:36: initial creation, out of the closure _CancelOn returned
//   - 2026-11-02 11:56: takes the hook and the step limit off the thread, where it waited for
//     a cancel under way
//   - 2026-11-04 00:30: clears the hook with _SetHook (C54 fix round 1, B093)
//   - 2026-11-06 00:02: does nothing for a watch never armed (C100a, B163)
//   - 2026-11-10 00:25: renamed from _Release, as a test in tests/ needs it (C120a, Q314)
func (w *Watch) Release() {
	var thread *starlark.Thread

	if w != nil {
		thread = w.Thread.Load()
	}

	if thread == nil {
		return
	}

	SetHook(
		thread,
		nil,
	)
	thread.SetMaxExecutionSteps(math.MaxUint64)
}

// Look is the hook the interpreter calls when the thread reaches its step
// limit. It cancels the thread if the context is done, and sets the next limit.
//
// The next limit is set whether or not the thread was cancelled. A hook that
// leaves the limit alone is called again on every step after it.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation, out of _Cancel
//   - 2026-11-06 00:02: a nil thread does nothing, and a nil watch or status never cancels
//     (C100a, B164)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:08: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-10 00:25: renamed from _Look, as a test in tests/ needs it (C120a, Q314)
func (w *Watch) Look(thread *starlark.Thread) {
	if thread == nil {
		return
	}

	if w != nil && w.Status != nil && w.Status() != nil {
		thread.Cancel(STOPPED)
	}

	thread.SetMaxExecutionSteps(thread.ExecutionSteps() + common.POLL)
}
