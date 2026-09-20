package scheduler

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/guard"
)

var (
	// ErrNotAName is every refusal spawn can make: the argument is missing, is
	// not a function, or takes parameters nothing can supply.
	//
	// A lambda is no longer among them. A compiler emits one wherever a call
	// passes arguments - spawn(lambda: greet("alice")) - because spawn takes no
	// arguments to pass on. What a handle lost by accepting it is its name, and
	// a name is not something a handle has to have: what reports a run is the
	// graph, which knows what runs on the lane a fork named.
	ErrNotAName = errors.New("spawn wants a function")

	// ErrCancelled separates "the interpreter stopped you" from "your code was
	// wrong", because a caller acts on them differently.
	ErrCancelled = errors.New("handle cancelled")

	// ErrDuration is returned for a duration that is not one.
	ErrDuration = errors.New("not a duration")
)

const (
	SPAWN = "spawn"

	// LAMBDA is what the interpreter calls an anonymous function. Nothing
	// refuses one; this is here so a reporter can tell that a name is not one.
	LAMBDA = "lambda"

	// REPORTER names what failed when a host's own reporter raises, so the
	// error says whose bug it is rather than reading as the script's.
	REPORTER = "reporter"
)

// _Spawn starts fn on a goroutine and an interpreter thread of its own, and
// hands the script back a handle to it.
//
// A named function only. A Starlark thread cannot be shared across goroutines,
// so a spawned call cannot borrow the caller's; frozen globals can be, which is
// what makes the new thread cheap. The name is taken here rather than at report
// time because a handle outlives the call that made it.
//
// Returns ErrNotAName when fn is missing, is not callable, or is a lambda.
//
// Revisions:
//   - 2026-09-19 20:38: initial creation
func _Spawn(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	run, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	target, err := _Named(args, kwargs)
	if err != nil {
		return nil, err
	}

	inner, stop := context.WithCancel(run.ctx)

	handle := &Handle{
		name: target.Name(),
		done: make(chan struct{}),
		stop: stop,
	}

	run._Track(handle)
	run.group.Add(1)

	run._Tell(func() {
		run.into.Started(handle.thread, handle.name, NO_ATTEMPT)
	})

	go func() {
		defer run.group.Done()
		defer close(handle.done)
		defer stop()

		handle._Work(inner, run, target)
	}()

	return handle, nil
}

// _Named returns the one named function in args, or says why it is not one.
//
// Revisions:
//   - 2026-09-19 20:39: initial creation
func _Named(args starlark.Tuple, kwargs []starlark.Tuple) (*starlark.Function, error) {
	if len(args) != 1 || len(kwargs) > 0 {
		return nil, fmt.Errorf("%s takes one function: %w", SPAWN, ErrNotAName)
	}

	target, ok := args[0].(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("%s got %s: %w", SPAWN, args[0].Type(), ErrNotAName)
	}

	if target.NumParams() > 0 {
		return nil, fmt.Errorf("%s got %s, which takes arguments: %w", SPAWN, target.Name(), ErrNotAName)
	}

	return target, nil
}

// _Work runs the spawned call on its own thread and records what it produced.
//
// The call is guarded, and this is the place where that matters most: a panic
// in a goroutine cannot be recovered from outside it, so an unguarded builtin
// that panics here would not fail the run - it would take the process down,
// host and all. Every other guard in this tree is tidiness; this one is why the
// package can be handed a script it did not write.
//
// The context cancels the thread, so a cancelled handle stops at the spawned
// evaluation's next instruction rather than running to completion unheeded. A
// failure that arrives with the context already done is reported as a
// cancellation, because "the interpreter stopped you" and "your code was wrong"
// are different answers and a caller acts on them differently.
//
// Revisions:
//   - 2026-09-19 20:40: initial creation
//   - 2026-09-19 23:24: guards the call, so a panicking builtin fails the handle
//     instead of killing the process
func (h *Handle) _Work(ctx context.Context, run *_Run, target *starlark.Function) {
	thread := &starlark.Thread{Name: h.name}
	thread.SetLocal(RUN_KEY, run)
	thread.SetLocal(THREAD_KEY, h.thread)
	thread.SetLocal(CONTEXT_KEY, ctx)
	thread.SetLocal(REPORTER_KEY, run.into)

	stop := _CancelOn(ctx, thread)
	defer stop()

	guard.WithRecover(
		&h.value,
		&h.err,
		func() (starlark.Value, error) {
			return starlark.Call(thread, target, nil, nil)
		},
	)

	raw := h.err

	if h.err != nil && ctx.Err() != nil {
		// The interpreter's own message also says "cancelled", so it is
		// replaced rather than wrapped: what a reader needs is which handle,
		// and why the context ended.
		//
		// No failure is exempt, including the one that caused the cancellation.
		// A handle reports what happened to that thread; the run's outcome
		// reports why the run ended. Keeping those apart is what lets this be
		// one rule with no cases in it.
		h.err = fmt.Errorf("%w: %w", ErrCancelled, ctx.Err())
	}

	h._Report(run, raw)
}

// _Report tells the run's reporter how this handle ended.
//
// The relabelled error, except for the one thread that caused the run to stop.
// Both halves are needed and neither is obvious.
//
// A thread stopped by the cancellation must report the relabelled error,
// because the interpreter raises its own cancellation from a reason string
// which wraps nothing - a reporter handed that cannot tell a thread that was
// stopped from one that broke, and in a fail-fast runtime most stopped threads
// were doing nothing wrong.
//
// The thread whose failure ended the run must report its own, because the
// relabel is deliberately unconditional: _Work says a handle reports what
// happened to that thread, and the run's outcome says why the run ended. That
// is right for a caller joining a handle, and wrong for a report of which
// function failed - it would show the one thing that did fail as cancelled,
// alongside every sibling it stopped.
//
// A handle is the cause when the run's outcome is reachable from its own raw
// error, which is exactly what _End recorded.
//
// Contained, because a reporter belongs to a host and a host's code panicking
// on a goroutine of ours would take the process down rather than fail anything.
// The panic ends the run, which is what phase 5 froze when it put the report
// inside the guard: a host's callback is host code, and host code that raises
// on a thread of ours is a bug somebody has to be told about. Absorbing it
// would leave a run reporting success while the report of it was never made.
//
// Revisions:
//   - 2026-09-20 11:35: initial creation
func (h *Handle) _Report(run *_Run, raw error) {
	ended := h.err

	outcome := run._Outcome()
	if outcome != nil && raw != nil && errors.Is(raw, outcome) {
		ended = raw
	}

	run._Tell(func() {
		run.into.Ended(h.thread, h.name, ended)
	})
}
