package scheduler

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// Poll makes the thread look at its context now. A thread with no watch is
// left alone.
//
// The step poll of _Arm looks only every common.POLL steps. A builtin
// that blocks, or runs long, can return after the context is done with the
// next poll still far off, and the script would then start another builtin.
// Poll closes that gap: it calls the watch's hook, which cancels the thread if
// the context is done and sets the next step limit common.POLL steps on. The
// interpreter reads the cancel before its next instruction runs, so no
// builtin starts after a poll sees the cancel.
//
// Why the hook and not a lookup of the context: the thread reaches its run
// through Thread.Local, a map with a string key. Profiled on a loop of
// 2,000,000 calls of math.ceil, that lookup was 95 percent of the cost of the
// first form of Poll (0.35 s of 0.37 s), and context.Err, an atomic load, 3
// percent. The hook is a field of the thread and holds the context itself.
//
// The hook is the mark of a watch: _Arm sets it and _Release clears it,
// and nothing else in the runtime uses OnMaxSteps.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-02 11:56: calls the thread's hook, in place of reading the context through
//     the thread's locals
//   - 2026-11-04 00:30: reads the hook once, so the one that is tested is the one that is
//     called (C54, B093)
//   - 2026-11-04 00:30: reads the hook with an atomic load, so a hook set from another
//     goroutine through _SetHook is read whole (C54 fix round 1, B093)
//   - 2026-11-04 23:50: does nothing for a nil thread (C86 fix round 3)
//   - 2026-11-07 20:10: forwards to run.Poll (C91w5)
func Poll(thread *starlark.Thread) {
	run.Poll(thread)
}

// Polled returns env with every builtin in it wrapped, so that each polls
// when it returns. It changes nothing in env.
//
// The seam. The interpreter has no hook on a call or a return, and its step
// limit counts steps, not calls. The one place every plugin's builtins pass
// through, whoever wrote the plugin, is the environment a compile is given:
// wrapping there needs no change to the 75 places that make a builtin, and
// none to a host's plugin. A builtin inside a module is wrapped too.
//
// Not wrapped: a value that is not a builtin or a module; a builtin the
// interpreter or a value makes later, such as the methods of a list or of an
// object a plugin returns; and the universe builtins. None of them has an
// effect outside the run, except print, which polls in the run's own print.
//
// The wrapper calls the builtin's own CallInternal, so the builtin sees
// itself as fn, the wrapper has its name, and one frame on the
// call stack, not two.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-07 20:10: forwards to run.Polled (C91w5)
func Polled(env starlark.StringDict) starlark.StringDict {
	return run.Polled(env)
}
