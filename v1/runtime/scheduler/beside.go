package scheduler

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// ErrCancelled separates "something stopped you" from "your code was wrong",
// because a caller acts on them differently.
//
// It covers both stops a caller can meet: a joined handle that was cancelled,
// which is a fact about one thread, and a run that was stopped from outside,
// which is the caller's own doing. One sentinel rather than two, decided
// 2026-09-23 06:58 - a host asking "did this finish on its own" wants one
// answer, and the two are told apart by what else the error carries.
var (
	ErrCancelled = errors.New("cancelled")
)

// Option sets one thing on the evaluation Beside starts, given the evaluation
// that is starting it.
type Option func(
	parent *_Locals,
	child *_Locals,
)

// Beside starts target on a goroutine and an interpreter thread of its own,
// inheriting everything the caller's evaluation carries, and returns the
// handle that finds its result.
//
// spawn calls it with no options and gets a lane of its own, numbered as a
// child of the caller's; a timeout calls it with Inline and gets an evaluation
// on the caller's own lane. The new thread's context descends from the
// caller's evaluation, not from the run, so a timeout around the caller
// reaches it. An attempt of a repeat or a retry runs Within instead.
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
//   - 2026-10-02 00:38: reports a spawn on the thread that wrote it, with the
//     child's id, its binding and the function it runs
func Beside(
	thread *starlark.Thread,
	target starlark.Callable,
	opts ...Option,
) (*Handle, error) {
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

	spawn := _BesideHandle(
		thread,
		parent,
		target,
		opts,
	)

	_BesideGo(spawn)

	return spawn.handle, nil
}

// _BesideCost charges the run for one more thread, before anything is built.
//
// A thread is the largest thing this library allocates for a script. Charged
// here, nothing has to be unwound when a run cannot afford another.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _BesideCost(
	parent *_Locals,
	target starlark.Callable,
) error {
	err := parent.run.budget.Charge(THREAD_COST)
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			target.Name(),
			err,
		)
	}

	return nil
}

// _Spawn is one child Beside is about to start.
type _Spawn struct {
	parent *_Locals
	handle *Handle
	made   *starlark.Thread
	child  *_Locals
	target starlark.Callable
	line   *Line
}

// _BesideHandle is the handle, the thread, and the locals a spawn starts with.
//
// line is set when this spawn numbered the child itself.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-24 19:30: a spawn that owns its lane derives from the run's
//     context, so it survives a wrapper's body returning
//   - 2026-10-28 22:36: derives the child's context once, from the parent it
//     keeps, rather than from the caller's and then again from the run's
func _BesideHandle(
	thread *starlark.Thread,
	parent *_Locals,
	target starlark.Callable,
	opts []Option,
) *_Spawn {
	var (
		child = _Inherited(
			parent,
			opts,
		)
		owns = child.thread == ""
		base = parent.ctx
	)

	// A spawn that owns its lane lives until the run ends, not until its
	// parent's evaluation ends. Without this, a spawn inside a wrapper is
	// cancelled when the wrapper's body returns, before the caller can join
	// it. An attempt or an inline still derives from the parent, so a
	// timeout's context reaches it.
	if owns {
		base = parent.run.ctx
		child.thread = parent.run._Number(parent.thread)
	}

	inner, stop := context.WithCancel(base)
	child.ctx = inner

	var (
		handle, made = _Made(
			thread,
			target,
			child,
			stop,
		)
		spawn = &_Spawn{
			parent: parent,
			handle: handle,
			made:   made,
			child:  child,
			target: target,
		}
	)

	if owns {
		spawn.line = &Line{
			Name:    handle.name,
			Builtin: spelling.SPAWN,
			Child:   handle.thread,
			Binding: child.binding,
		}
	}

	return spawn
}

// _Made is the handle and the thread a spawn runs on.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _Made(
	thread *starlark.Thread,
	target starlark.Callable,
	child *_Locals,
	stop context.CancelFunc,
) (*Handle, *starlark.Thread) {
	var (
		handle = &Handle{
			name: Named(
				target,
				child.label,
			),
			thread: child.thread,
			done:   make(chan struct{}),
			stop:   stop,
		}
		made = &starlark.Thread{
			Name:  target.Name(),
			Print: thread.Print,
		}
	)

	made.SetLocal(
		LOCALS_KEY,
		child,
	)

	return handle, made
}

// _BesideGo starts the child and reports the spawn on the parent.
//
// A spawn is a line of the thread that wrote it. The child does not report
// its own entry again. The cost is credited when the thread ends.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _BesideGo(spawn *_Spawn) {
	var (
		parent = spawn.parent
		handle = spawn.handle
	)

	if spawn.line != nil {
		line := spawn.line

		parent.run._Tell(func() {
			parent.run.into.Started(
				parent.thread,
				line,
			)
		})
	}

	parent.run.group.Add(1)

	go func() {
		defer parent.run.budget.Credit(THREAD_COST)
		defer parent.run.group.Done()
		defer close(handle.done)
		defer handle.stop()

		handle._Work(
			spawn.made,
			spawn.child,
			spawn.target,
		)

		if spawn.line != nil {
			handle._Report(parent.run)
		}
	}()
}

// _Inherited is the locals a child evaluation starts with, before it has a
// context.
//
// No context here, because which parent the context derives from depends on
// the lane the options choose. Deriving one before that was known meant
// deriving a second for every spawn that owns its lane.
//
// Revisions:
//   - 2026-09-27 01:34: initial creation, lifted out of Beside
//   - 2026-10-24 18:29: takes the context first
//   - 2026-10-28 22:36: takes no context, the caller setting it once the
//     options have chosen the lane
func _Inherited(
	parent *_Locals,
	opts []Option,
) *_Locals {
	child := &_Locals{
		run:      parent.run,
		attempt:  parent.attempt,
		catching: parent.catching,
		inside:   parent.inside,

		// Never inherited. The lock belongs to the evaluation that took it, so a
		// child carries the fact that one is held without being the one holding
		// it - which is what lets the refusal say which of the two it is
		// refusing.
		holding: false,
	}

	for _, opt := range opts {
		opt(
			parent,
			child,
		)
	}

	return child
}

// Labelled tells Beside what the thread a spawn starts runs, and the name the
// script bound the spawn to, for the line it reports.
//
// The function is the one a lambda calls, which the dialect read off the
// source: a lambda is only how a spawn passes arguments, and its own name says
// nothing. Empty when the lambda calls no function the script defines.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation
func Labelled(
	name string,
	binding string,
) Option {
	return func(
		parent *_Locals,
		child *_Locals,
	) {
		child.label = name
		child.binding = binding
	}
}

// Named is the name a line reports for a call of target: the callee the
// dialect read off the source, or target's own name, except that a lambda with
// no callee reports none, since "lambda" would read like a function of that
// name.
//
// One rule for every builtin that runs a function it was handed. A lambda is
// only how a spawn or a wrapper passes arguments, so the function it calls is
// what its line names.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation, as _Named
//   - 2026-10-02 00:59: exported, taking the callee rather than the locals, so
//     a wrapper names its line the way a spawn does
func Named(
	target starlark.Callable,
	callee string,
) string {
	if callee != "" || target.Name() == LAMBDA {
		return callee
	}

	return target.Name()
}

// Attempt marks the evaluation as attempt n of a repeat or a retry, on the
// caller's own lane: an attempt is the caller's step, not a thread of its own.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func Attempt(n int32) Option {
	return func(
		parent *_Locals,
		child *_Locals,
	) {
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
	return func(
		parent *_Locals,
		child *_Locals,
	) {
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
	return func(
		parent *_Locals,
		child *_Locals,
	) {
		child.catching++
	}
}

// _Work runs the call on its own thread, watching the evaluation's context,
// and records what it produced.
//
// The thread is the evaluation's own, and nothing else watches it. The watch
// is how a cancel of the evaluation reaches it.
//
// Revisions:
//   - 2026-09-19 20:40: initial creation
//   - 2026-09-19 23:24: guards the call, so a panicking builtin fails the handle
//     instead of killing the process
//   - 2026-09-21 08:09: takes the evaluation it runs as, and keeps the raw
//     error for the report
//   - 2026-10-03 23:52: ends with a failure Fail kept on the evaluation, when what it
//     returned does not already carry it
//   - 2026-10-24 20:00: ends the run on a recovered panic, so an unjoined panic
//     does not vanish
//   - 2026-10-29 00:39: calls through _Evaluate, which Within shares, and keeps
//     only the watch
func (h *Handle) _Work(
	thread *starlark.Thread,
	locals *_Locals,
	target starlark.Callable,
) {
	watching := _CancelOn(
		locals.ctx,
		thread,
	)
	defer watching._Release()

	h._Evaluate(
		thread,
		locals,
		target,
	)
}

// _Evaluate calls target on thread as the evaluation locals describes,
// guarded, and records on h what it produced.
//
// A failure that arrives with the context already done is reported as a
// cancellation, because "the interpreter stopped you" and "your code was
// wrong" are different answers and a caller acts on them differently. No
// failure is exempt, including the one that caused the cancellation: a handle
// reports what happened to that thread, and the run's outcome reports why the
// run ended.
//
// A panic ends the run whether or not the handle is joined, because a panic
// is always a bug and silence is the wrong answer for a bug. The call sets
// called to true on the normal path; a panic never reaches that line, so
// called stays false and the run is ended with the recovered error.
//
// Revisions:
//   - 2026-10-29 00:39: initial creation, out of _Work, so an evaluation on
//     its caller's thread ends as one on a thread of its own does
func (h *Handle) _Evaluate(
	thread *starlark.Thread,
	locals *_Locals,
	target starlark.Callable,
) {
	var called bool

	guard.WithRecover(
		&h.value,
		&h.err,
		func() (starlark.Value, error) {
			result, err := starlark.Call(
				thread,
				target,
				nil,
				nil,
			)
			called = true

			return result, err
		},
	)

	// A failure Fail kept is what this evaluation ended with, whether it went
	// on to return nothing - a loop over a file cut short - or to raise the
	// cancellation the run's stop then sent it. One the evaluation raised
	// itself, as assert does, is already what it returned.
	kept := locals.failed != nil && !errors.Is(
		h.err,
		locals.failed,
	)
	if kept {
		h.value = nil
		h.err = locals.failed
	}

	h.raw = h.err

	// A panic is always a bug. End the run so the error reaches the caller
	// whether or not anybody joins this handle.
	if !called && h.raw != nil {
		locals.run._End(h.raw)
	}

	if h.err != nil && locals.ctx.Err() != nil {
		h.err = fmt.Errorf(
			"%w: %w",
			ErrCancelled,
			locals.ctx.Err(),
		)
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
	var (
		ended   = h.err
		outcome = run._Outcome()
	)

	if outcome != nil && h.raw != nil && errors.Is(
		h.raw,
		outcome,
	) {
		ended = h.raw
	}

	run._Tell(func() {
		run.into.Ended(
			h.thread,
			h.name,
			ended,
		)
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
func Wait(
	thread *starlark.Thread,
	handle *Handle,
) (starlark.Value, error) {
	locals, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	select {
	case <-handle.done:
		return handle.value, handle.err

	case <-locals.ctx.Done():
		return nil, fmt.Errorf(
			"%w: %w",
			ErrCancelled,
			locals.ctx.Err(),
		)
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
func Settle(
	thread *starlark.Thread,
	handle *Handle,
) {
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
