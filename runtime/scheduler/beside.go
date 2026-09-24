package scheduler

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/guard"
)

// ErrCancelled separates "something stopped you" from "your code was wrong",
// because a caller acts on them differently.
//
// It covers both stops a caller can meet: a joined handle that was cancelled,
// which is a fact about one thread, and a run that was stopped from outside,
// which is the caller's own doing. One sentinel rather than two, decided
// 2026-09-23 06:58 - a host asking "did this finish on its own" wants one
// answer, and the two are told apart by what else the error carries.
var ErrCancelled = errors.New("cancelled")

// Option sets one thing on the evaluation Beside starts, given the evaluation
// that is starting it.
type Option func(parent *_Locals, child *_Locals)

// Beside starts target on a goroutine and an interpreter thread of its own,
// inheriting everything the caller's evaluation carries, and returns the
// handle that finds its result.
//
// This is the one primitive. spawn calls it with no options and gets a lane of
// its own, numbered as a child of the caller's; a wrapper calls it with
// Attempt or Inline and gets an evaluation on the caller's own lane. The new
// thread's context descends from the caller's evaluation, not from the run,
// so a timeout around the caller reaches it.
//
// A lane reports itself starting and ending; an evaluation on the caller's
// lane does not, because its wrapper reports once for however many attempts
// it makes.
//
// The call is guarded, and this is the place where that matters most: a panic
// on a goroutine cannot be recovered from outside it, so an unguarded call
// here would not fail the run - it would take the process down, host and all.
//
// Returns ErrNoRun when thread carries no run.
//
// Revisions:
//   - 2026-09-19 20:38: initial creation, as _Spawn's body
//   - 2026-09-21 08:09: one primitive for spawn and every wrapper, inheriting
//     the caller's evaluation and deriving from its context
func Beside(thread *starlark.Thread, target starlark.Callable, opts ...Option) (*Handle, error) {
	parent, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	inner, stop := context.WithCancel(parent.ctx)

	child := &_Locals{
		run:      parent.run,
		ctx:      inner,
		attempt:  parent.attempt,
		catching: parent.catching,
		inside:   parent.inside,

		// Never inherited. The lock belongs to the evaluation that took it,
		// so a child carries the fact that one is held without being the one
		// holding it - which is what lets the refusal say which of the two it
		// is refusing.
		holding: false,
	}

	for _, opt := range opts {
		opt(parent, child)
	}

	owns := child.thread == ""
	if owns {
		child.thread = parent.run._Number(parent.thread)
	}

	handle := &Handle{
		name:   target.Name(),
		thread: child.thread,
		done:   make(chan struct{}),
		stop:   stop,
	}

	made := &starlark.Thread{Name: handle.name, Print: thread.Print}
	made.SetLocal(LOCALS_KEY, child)

	if owns {
		parent.run._Tell(func() {
			parent.run.into.Started(handle.thread, handle.name, NO_ATTEMPT)
		})
	}

	parent.run.group.Add(1)

	go func() {
		defer parent.run.group.Done()
		defer close(handle.done)
		defer stop()

		handle._Work(made, child, target)

		if owns {
			handle._Report(parent.run)
		}
	}()

	return handle, nil
}

// Attempt marks the evaluation as attempt n of a repeat or a retry, on the
// caller's own lane: an attempt is the caller's step, not a thread of its own.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func Attempt(n int32) Option {
	return func(parent *_Locals, child *_Locals) {
		child.attempt = n
		child.thread = parent.thread
	}
}

// Inline keeps the evaluation on the caller's lane without counting it as an
// attempt, which is what a timeout wants: one call, bounded, not numbered.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func Inline() Option {
	return func(parent *_Locals, child *_Locals) {
		child.thread = parent.thread
	}
}

// Catching marks the evaluation as catching assertions, one level deeper than
// the caller's.
//
// An assertion stops the run when nothing catches it, and fails only the
// attempt when something does. That is what lets retry exist without a script
// gaining try or except. A depth rather than a flag, so a retry inside a retry
// is still inside one when the inner finishes.
//
// Revisions:
//   - 2026-09-20 01:00: initial creation, as Catch
//   - 2026-09-21 08:09: an option on Beside rather than a mark set afterwards
func Catching() Option {
	return func(parent *_Locals, child *_Locals) {
		child.catching++
	}
}

// _Work runs the call on its own thread, guarded, and records what it
// produced.
//
// A failure that arrives with the context already done is reported as a
// cancellation, because "the interpreter stopped you" and "your code was
// wrong" are different answers and a caller acts on them differently. No
// failure is exempt, including the one that caused the cancellation: a handle
// reports what happened to that thread, and the run's outcome reports why the
// run ended.
//
// Revisions:
//   - 2026-09-19 20:40: initial creation
//   - 2026-09-19 23:24: guards the call, so a panicking builtin fails the handle
//     instead of killing the process
//   - 2026-09-21 08:09: takes the evaluation it runs as, and keeps the raw
//     error for the report
func (h *Handle) _Work(thread *starlark.Thread, locals *_Locals, target starlark.Callable) {
	watching := _CancelOn(locals.ctx, thread)
	defer watching()

	guard.WithRecover(
		&h.value,
		&h.err,
		func() (starlark.Value, error) {
			return starlark.Call(thread, target, nil, nil)
		},
	)

	h.raw = h.err

	if h.err != nil && locals.ctx.Err() != nil {
		h.err = fmt.Errorf("%w: %w", ErrCancelled, locals.ctx.Err())
	}
}

// _Report tells the run's reporter how this lane ended.
//
// The relabelled error, except for the one thread that caused the run to stop.
// A thread stopped by the cancellation must report the relabelled error,
// because in a fail-fast runtime most stopped threads were doing nothing
// wrong. The thread whose failure ended the run must report its own, or the
// report would show the one thing that did fail as cancelled, alongside every
// sibling it stopped. A handle is the cause when the run's outcome is
// reachable from its own raw error, which is exactly what _End recorded.
//
// Revisions:
//   - 2026-09-20 11:35: initial creation
//   - 2026-09-21 08:09: reads the raw error from the handle
func (h *Handle) _Report(run *_Run) {
	ended := h.err

	outcome := run._Outcome()
	if outcome != nil && h.raw != nil && errors.Is(h.raw, outcome) {
		ended = h.raw
	}

	run._Tell(func() {
		run.into.Ended(h.thread, h.name, ended)
	})
}

// Wait blocks until handle is over, or until the caller's own evaluation is
// cancelled, and returns what the handle produced.
//
// Waiting on the caller's context is what every blocking builtin owes. A join
// blocked on a handle from another lineage would otherwise hold a cancelled
// evaluation open for as long as that handle ran, which is how a timeout
// around a join used to wait the join out.
//
// Returns ErrCancelled wrapping the context's error when the caller is
// cancelled first.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func Wait(thread *starlark.Thread, handle *Handle) (starlark.Value, error) {
	locals, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	select {
	case <-handle.done:
		return handle.value, handle.err

	case <-locals.ctx.Done():
		return nil, fmt.Errorf("%w: %w", ErrCancelled, locals.ctx.Err())
	}
}

// Settle waits for handle to be over, or for the caller's evaluation to be
// cancelled, and says nothing about either.
//
// For a caller that has already stopped the handle and only needs it gone
// before returning: a cancelled handle has nothing to say, so there is no
// result to discard.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation, as _Settle for join's abort
//   - 2026-09-21 09:46: exported, since join lives in a plugin now
func Settle(thread *starlark.Thread, handle *Handle) {
	locals, err := _Of(thread)
	if err != nil {
		<-handle.done

		return
	}

	select {
	case <-handle.done:
	case <-locals.ctx.Done():
	}
}
