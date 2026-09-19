package scheduler

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/guard"
)

var (
	// ErrNotAName is every refusal spawn can make: the argument is missing,
	// is not a function, is a lambda, or takes parameters nothing can supply.
	ErrNotAName = errors.New("scheduler: spawn wants a named function")

	// ErrCancelled separates "the interpreter stopped you" from "your code was
	// wrong", because a caller acts on them differently.
	ErrCancelled = errors.New("scheduler: handle cancelled")
)

const (
	SPAWN  = "spawn"
	LAMBDA = "lambda"
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

	if target.Name() == LAMBDA {
		return nil, fmt.Errorf("%s got a lambda: %w", SPAWN, ErrNotAName)
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

	stop := _CancelOn(ctx, thread)
	defer stop()

	guard.WithRecover(
		&h.value,
		&h.err,
		func() (starlark.Value, error) {
			return starlark.Call(thread, target, nil, nil)
		},
	)

	if h.err != nil && ctx.Err() != nil {
		h.err = fmt.Errorf("%w: %w", ErrCancelled, h.err)
	}
}
