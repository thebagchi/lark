package scheduler

import (
	"go.starlark.net/starlark"
)

// Within runs target on the caller's own goroutine and interpreter thread, as
// an evaluation of its own, and returns what it produced.
//
// An attempt of a repeat or a retry needs no goroutine of its own: its caller
// does nothing but wait for it. A goroutine, a thread, a context and its
// watch were most of what an attempt cost. So the evaluation borrows the
// caller's thread and carries its own locals there while it runs. The
// caller's locals go back when it ends, however it ends.
//
// It ends as an evaluation Beside starts does, through _Evaluate: with a
// failure Fail kept, a panic that ends the run, and a failure under a cancel
// reported as cancelled.
//
// Its context is the caller's. A cancel of the caller is then a cancel of
// this evaluation, and the caller's thread already watches for it. Nothing
// can cancel this evaluation alone: a cancel of an interpreter thread lasts,
// so it would stop the caller as well. An evaluation that its caller must be
// able to stop, as a timeout's must, runs Beside.
//
// Charged as a thread is, though none is made, so a run holds an attempt to
// the budget it held a thread to.
//
// Returns ErrNoRun when thread carries no run, and ErrMemory when the run
// cannot afford the evaluation.
//
// Revisions:
//   - 2026-10-29 00:39: initial creation
func Within(
	thread *starlark.Thread,
	target starlark.Callable,
	opts ...Option,
) (starlark.Value, error) {
	parent, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	err = _BesideCost(
		parent,
		target,
	)
	if err != nil {
		return nil, err
	}
	defer parent.run.budget.Credit(THREAD_COST)

	child := _Inherited(
		parent,
		opts,
	)
	child.thread = parent.thread
	child.ctx = parent.ctx

	thread.SetLocal(
		LOCALS_KEY,
		child,
	)
	defer thread.SetLocal(
		LOCALS_KEY,
		parent,
	)

	ended := &Handle{}
	ended._Evaluate(
		thread,
		child,
		target,
	)

	return ended.value, ended.err
}
