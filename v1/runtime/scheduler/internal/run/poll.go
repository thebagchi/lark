package run

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Poll makes the thread look at its context now. A thread with no watch is
// left alone.
//
// The step poll of Arm looks only every common.POLL steps. A builtin
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
// The hook is the mark of a watch: Arm sets it and Release clears it,
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
//   - 2026-11-10 01:00: comment now names Arm, Release, were _Arm, _Release (C120a, Q314)
func Poll(thread *starlark.Thread) {
	if thread == nil {
		return
	}

	hook := _Hook(thread)
	if hook == nil {
		return
	}

	hook(thread)
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
func Polled(env starlark.StringDict) starlark.StringDict {
	wrapped := make(
		starlark.StringDict,
		len(env),
	)

	for name, value := range env {
		wrapped[name] = _Wrapped(value)
	}

	return wrapped
}

// _Wrapped is value with the builtins in it wrapped: a builtin, or the members
// of a module. Any other value is returned as it is.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
func _Wrapped(value starlark.Value) starlark.Value {
	switch typed := value.(type) {
	case *starlark.Builtin:
		return _Wrapping(typed)
	case *starlarkstruct.Module:
		return &starlarkstruct.Module{
			Name:    typed.Name,
			Members: Polled(typed.Members),
		}
	}

	return value
}

// _Wrapping is a builtin that calls inner, and polls when it returns.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-06 00:02: a nil inner gives a builtin that returns ErrNoCallee when called (C100a,
//     B166)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func _Wrapping(inner *starlark.Builtin) *starlark.Builtin {
	if inner == nil {
		return starlark.NewBuiltin(
			"",
			func(
				_ *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				return nil, fmt.Errorf(
					"nothing to call: %w",
					ErrRun,
				)
			},
		)
	}

	return starlark.NewBuiltin(
		inner.Name(),
		func(
			thread *starlark.Thread,
			_ *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			value, err := inner.CallInternal(
				thread,
				args,
				kwargs,
			)

			Poll(thread)

			return value, err
		},
	)
}

// _Hook is the thread's step hook, read with an atomic load.
//
// A func value is one pointer, so the load is of that pointer. The interpreter
// itself reads the field with no atomic, on the goroutine that runs the thread,
// which is the only reader besides Poll.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation, so Poll and _SetHook never race (C54 fix round 1, B093)
func _Hook(thread *starlark.Thread) func(*starlark.Thread) {
	loaded := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&thread.OnMaxSteps)))

	return *(*func(*starlark.Thread))(unsafe.Pointer(&loaded))
}

// SetHook sets the thread's step hook with an atomic store, or clears it when
// hook is nil.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation, so Poll and _SetHook never race (C54 fix round 1, B093)
//   - 2026-11-10 00:25: renamed from _SetHook, as a test in tests/ needs it (C120a, Q314)
func SetHook(
	thread *starlark.Thread,
	hook func(*starlark.Thread),
) {
	atomic.StorePointer(
		(*unsafe.Pointer)(unsafe.Pointer(&thread.OnMaxSteps)),
		*(*unsafe.Pointer)(unsafe.Pointer(&hook)),
	)
}
