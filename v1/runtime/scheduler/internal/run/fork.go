package run

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/internal/nilcheck"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// ErrCancelled separates "something stopped you" from "your code was wrong",
// because a caller acts on them differently.
//
// It covers both stops a caller can meet: a joined handle that was cancelled,
// which is a fact about one thread, and a run that was stopped from outside,
// which is the caller's own doing. One sentinel rather than two, decided
// 2026-09-23 06:58 - a host asking "did this finish on its own" wants one
// answer, and the two are told apart by what else the error carries.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
var (
	ErrCancelled = errors.New("cancelled")
)

// Fork starts target on a goroutine and an interpreter thread of its own,
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
// A spawn that owns a lane is refused once the run is stopped, whatever stopped
// it: a failure, the caller's context, the memory watch. No thread starts and
// nothing is reported for it, so a stopped run does no new work. A graph shows
// the spawn that was not made from the script (Q18). An evaluation on the
// caller's lane is not refused: its context is the caller's, and the caller
// is stopped by then or soon.
//
// Returns script.ErrInit, before anything starts, when thread is running the
// top level of a module: the new thread would read the globals the top level
// still writes (Q383, Q389). The text names the builtin that called Fork, as
// the innermost frame of the thread says, and gives no name when no builtin is
// running.
//
// Returns ErrRun when thread carries no run or target is nil, and ErrCancelled
// wrapping the run's context error for a spawn made after the run stopped. A nil
// option is skipped.
//
// Revisions:
//   - 2026-09-19 20:38: initial creation, as _Spawn's body
//   - 2026-09-21 08:09: one primitive for spawn and every wrapper, inheriting
//     the caller's evaluation and deriving from its context
//   - 2026-10-02 00:38: reports a spawn on the thread that wrote it, with the
//     child's id, its binding and the function it runs
//   - 2026-11-01 13:00: charges no thread cost, the budget being gone
//   - 2026-11-02 15:48: starts the spawn and its flight, and returns the handle
//     inside the spawn
//   - 2026-11-04 23:50: a nil target is ErrNoCallee, and a nil option is skipped
//     (C86 fix round 3)
//   - 2026-11-05 00:27: refuses a spawn that owns a lane once the run is
//     stopped (C45, Q18)
//   - 2026-11-07 19:56: takes common.BesideOption values and returns a common.Handle (C91w5, Q119,
//     Q127)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Of, BesideHandle, BesideGo, were _Of, _BesideHandle, _BesideGo
//     (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
//   - 2026-11-11 00:40: finds a nil target with nilcheck.Absent (C122, B268)
//   - 2026-11-12 17:20: refuses a call made while the module's top level runs (C128, B278, Q389)
//   - 2026-11-12 18:58: renamed from Beside (C129, Q396)
func Fork(
	thread *starlark.Thread,
	target starlark.Callable,
	opts ...common.ForkOption,
) (common.Handle, error) {
	if script.Loads(thread) {
		return nil, script.Loading(_Frame(thread))
	}

	parent, err := Of(thread)
	if err != nil {
		return nil, err
	}

	if nilcheck.Absent(target) {
		return nil, fmt.Errorf(
			"nothing to call: %w",
			ErrRun,
		)
	}

	spawn, flight, err := ForkHandle(
		thread,
		parent,
		target,
		opts,
	)
	if err != nil {
		return nil, err
	}

	ForkGo(
		spawn,
		flight,
	)

	return &spawn.Handle, nil
}

// _Frame is the name of the builtin that is running on thread: the innermost
// frame of its call stack. It is empty when no frame is on the stack.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B278)
func _Frame(thread *starlark.Thread) string {
	if thread == nil || thread.CallStackDepth() == 0 {
		return ""
	}

	return thread.CallFrame(0).Name
}

// Spawn is what a spawn keeps for as long as its handle lives, in one
// allocation: the handle a script holds and the locals the child evaluates
// with.
//
// A handle a script keeps points into this struct, so it keeps all of it alive.
// _Finish therefore lets go of every reference the struct holds once the thread
// is over, so a kept handle holds its result and its own fields, and neither
// the thread nor the run. What only the goroutine needs is in Flight, which
// the handle does not reach. That keeps this struct small, and its size is
// what a kept handle costs.
//
// The two are embedded, not named fields: the convention check refuses a named
// field that holds a module struct by value, and this struct exists to hold
// both by value.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: renamed from _Spawn and the fields _Locals, as a test in tests/ needs them
//     (C120a, Q314)
type Spawn struct {
	Handle
	Locals
}

// Flight is what a spawn's goroutine needs and a kept handle does not, in one
// allocation: the run and lane of the parent, the function to run, the
// interpreter thread, the line the spawn reports and the watch the thread keeps
// on its context.
//
// run and lane are the parent's, copied: a child that outlives its parent must
// not read the parent's locals, which _Finish clears. The goroutine's closure
// is the only holder of a Flight, so it is garbage when the goroutine ends.
// Line is embedded for the reason Spawn embeds its structs.
//
// Revisions:
//   - 2026-11-02 16:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: renamed from _Flight, as a test in tests/ needs it (C120a, Q314)
type Flight struct {
	run    *Run
	lane   string
	target starlark.Callable
	owns   bool
	made   starlark.Thread
	common.Line
	Watch
}

// ForkHandle is the spawn and the flight, with the handle, thread and
// locals made.
//
// owns is set when this spawn numbered the child itself.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-24 19:30: a spawn that owns its lane derives from the run's
//     context, so it survives a wrapper's body returning
//   - 2026-10-28 22:36: derives the child's context once, from the parent it
//     keeps, rather than from the caller's and then again from the run's
//   - 2026-11-02 12:05: one allocation holds the handle and locals, and a
//     _Flight holds the rest
//   - 2026-11-05 00:27: returns ErrCancelled for a spawn that owns its lane
//     when the run is stopped, before it numbers a child or derives a context
//     (C45, Q18)
//   - 2026-11-05 08:20: a spawn that owns its lane derives from a shard of the
//     run's context, which spreads the mutex its ending threads wait for (C53, Q67)
//   - 2026-11-06 00:02: returns ErrNoRun for a parent with no run, where it dereferenced one
//     (C100a, B156, B157)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: takes []common.BesideOption (C91w5, Q119)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 19:23: reads the name and thread through their methods (C112, Q207)
//   - 2026-11-10 00:25: renamed from _BesideHandle, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
//   - 2026-11-12 18:58: renamed from BesideHandle (C129, Q396)
func ForkHandle(
	thread *starlark.Thread,
	parent *Locals,
	target starlark.Callable,
	opts []common.ForkOption,
) (*Spawn, *Flight, error) {
	if parent == nil || parent.Run == nil {
		return nil, nil, ErrRun
	}

	var (
		spawn  = new(Spawn)
		flight = &Flight{
			run:    parent.Run,
			lane:   parent.ThreadID,
			target: target,
		}
		child = &spawn.Locals
		base  = parent.Ctx
	)

	Inherit(
		parent,
		child,
		opts,
	)

	flight.owns = child.ThreadID == ""

	if flight.owns {
		stopped := parent.Run.Ctx.Err()
		if stopped != nil {
			return nil, nil, fmt.Errorf(
				"%s: %w: %w",
				common.SPAWN,
				ErrCancelled,
				stopped,
			)
		}
	}

	// A spawn that owns its lane lives until the run ends, not until its
	// parent's evaluation ends. Without this, a spawn inside a wrapper is
	// cancelled when the wrapper's body returns, before the caller can join
	// it. An attempt or an inline still derives from the parent, so a
	// timeout's context reaches it.
	if flight.owns {
		base = parent.Run.Parent()
		child.ThreadID = parent.Run._Number(parent.ThreadID)
	}

	inner, stop := context.WithCancel(base)
	child.Ctx = inner

	spawn._Make(
		flight,
		thread,
		stop,
	)

	if flight.owns {
		flight.Line = common.Line{
			Name:    spawn.Name(),
			Builtin: common.SPAWN,
			Child:   spawn.Thread(),
			Binding: child.binding,
		}
	}

	return spawn, flight, nil
}

// _Make fills the handle and the thread a spawn runs on.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation, as _Made
//   - 2026-11-02 12:05: fills the handle in the spawn and the thread in the
//     flight instead of returning them
//   - 2026-11-04 00:30: stores the stop through _Hold (C54 fix round 1, B091)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:08: takes []common.BesideOption (C91w5, Q119)
//   - 2026-11-08 19:23: sets the name, thread and channel through the _Set methods (C112, Q207)
//   - 2026-11-10 01:00: text follows the renames (C120a, Q314)
func (s *Spawn) _Make(
	flight *Flight,
	thread *starlark.Thread,
	stop context.CancelFunc,
) {
	s.SetName(Named(
		flight.target,
		s.label,
	))
	s.SetThread(s.ThreadID)
	s.SetDone(make(chan struct{}))
	s.Hold(stop)

	flight.made.Name = flight.target.Name()
	flight.made.Print = thread.Print
	flight.made.SetLocal(
		LOCALS_KEY,
		&s.Locals,
	)
}

// ForkGo starts the child and reports the spawn on the parent.
//
// A spawn is a line of the thread that wrote it. The child does not report
// its own entry again. The cost is credited when the thread ends.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 00:19: the wait group's Go starts the goroutine and counts it,
//     so the count is released after the deferred cleanup, as Done was
//   - 2026-11-02 12:05: takes the parent's run and lane in a _Flight
//   - 2026-11-06 00:02: starts nothing for a nil spawn, flight or run (C100a, B158)
//   - 2026-11-08 19:23: reads the channel through _Chan (C112, Q207)
//   - 2026-11-10 00:25: renamed from _BesideGo, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-12 18:58: renamed from BesideGo (C129, Q396)
func ForkGo(
	spawn *Spawn,
	flight *Flight,
) {
	if spawn == nil || flight == nil || flight.run == nil {
		return
	}

	var (
		run    = flight.run
		lane   = flight.lane
		handle = &spawn.Handle
	)

	if flight.owns {
		line := &flight.Line

		run._Tell(func() {
			run.into.Started(
				lane,
				line,
			)
		})
	}

	run.Group.Go(func() {
		defer close(handle.Chan())
		defer spawn._Finish()

		raw := handle.Work(
			&flight.made,
			&spawn.Locals,
			flight.target,
			&flight.Watch,
		)

		if flight.owns {
			handle.Report(
				run,
				raw,
			)
		}
	})
}

// _Finish stops the evaluation's context and lets go of everything the spawn
// held to run, leaving the handle with its name, thread, channel and result.
//
// Runs before done closes, so a joiner that sees done sees the handle already
// clear. Stop takes the same mutex, so a Stop that races the finish either
// runs the stop or finds none; neither reads a half-cleared handle. The line
// is in the flight, so a reporter that keeps the pointer it was given reads a
// line that stays whole.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation, for the Q13 decision of change C33
//   - 2026-11-10 01:00: now uses Spawn, Detach, Locals, were _Spawn, _Detach, _Locals
//     (C120a, Q314)
func (s *Spawn) _Finish() {
	stop := s.Detach()
	stop()

	s.Locals = Locals{}
}

// Inherited is the locals a child evaluation starts with, before it has a
// context.
//
// Revisions:
//   - 2026-09-27 01:34: initial creation, lifted out of Beside
//   - 2026-10-24 18:29: takes the context first
//   - 2026-10-28 22:36: takes no context, the caller setting it once the
//     options have chosen the lane
//   - 2026-11-02 12:05: fills the locals _Inherit is handed
//   - 2026-11-06 00:02: a nil parent gives locals with no run, as _Inherit reads it (C100a, B159)
//   - 2026-11-07 20:07: takes []common.BesideOption (C91w5, Q119)
//   - 2026-11-10 00:25: renamed from _Inherited, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func Inherited(
	parent *Locals,
	opts []common.ForkOption,
) *Locals {
	child := new(Locals)

	Inherit(
		parent,
		child,
		opts,
	)

	return child
}

// Inherit fills child with what a child evaluation of parent starts with,
// before it has a context.
//
// No context here, because which parent the context derives from depends on
// the lane the options choose. Deriving one before that was known meant
// deriving a second for every spawn that owns its lane.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation, out of _Inherited, so a spawn can
//     fill the locals it holds
//   - 2026-11-06 00:02: does nothing for a nil child, and reads a nil parent as one with no run
//     (C100a, B159, B160)
//   - 2026-11-07 19:56: hands each option the two locals as common.Evaluation values, made only
//     when
//     there is an option (C91w5, Q119)
//   - 2026-11-10 00:25: renamed from _Inherit, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func Inherit(
	parent *Locals,
	child *Locals,
	opts []common.ForkOption,
) {
	if child == nil {
		return
	}

	if parent == nil {
		parent = new(Locals)
	}

	*child = Locals{
		Run:      parent.Run,
		attempt:  parent.attempt,
		Catching: parent.Catching,
		inside:   parent.inside,

		// Never inherited. The lock belongs to the evaluation that took it, so a
		// child carries the fact that one is held without being the one holding
		// it - which is what lets the refusal say which of the two it is
		// refusing.
		holding: false,
	}

	if len(opts) == 0 {
		return
	}

	var (
		from = &Evaluation{
			locals: parent,
		}
		to = &Evaluation{
			locals: child,
		}
	)

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		opt(
			from,
			to,
		)
	}
}

// _Pair reads the locals of the two evaluations a ForkOption is given. It
// reports false when either one is not an Evaluation of this package, so an
// option handed anything else does nothing.
//
// Revisions:
//   - 2026-11-07 19:56: initial creation, since an option now receives common.Evaluation (C91w5,
//     Q119, Q128)
//   - 2026-11-10 01:00: now uses Locals, were _Locals (C120a, Q314)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func _Pair(
	parent common.Evaluation,
	child common.Evaluation,
) (*Locals, *Locals, bool) {
	from, ok := parent.(*Evaluation)
	if !ok || from == nil {
		return nil, nil, false
	}

	to, ok := child.(*Evaluation)
	if !ok || to == nil {
		return nil, nil, false
	}

	return from.locals, to.locals, true
}

// Labelled tells Fork what the thread a spawn starts runs, and the name the
// script bound the spawn to, for the line it reports.
//
// The function is the one a lambda calls, which the dialect read off the
// source: a lambda is only how a spawn passes arguments, and its own name says
// nothing. Empty when the lambda calls no function the script defines.
//
// A child that is nil has nothing to label, and the option does nothing.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation
//   - 2026-11-04 00:30: does nothing for a nil child (C54, B085)
//   - 2026-11-07 19:56: takes and sets locals through common.Evaluation (C91w5, Q119)
//   - 2026-11-12 18:58: uses Fork and ForkOption, formerly Beside and BesideOption (C129, Q396)
func Labelled(
	name string,
	binding string,
) common.ForkOption {
	return func(
		parent common.Evaluation,
		child common.Evaluation,
	) {
		_, to, ok := _Pair(
			parent,
			child,
		)
		if !ok || to == nil {
			return
		}

		to.label = name
		to.binding = binding
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
// A nil target has no name of its own, so the callee is all there is.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation, as _Named
//   - 2026-10-02 00:59: exported, taking the callee rather than the locals, so
//     a wrapper names its line the way a spawn does
//   - 2026-11-04 00:30: a nil target names nothing, so the callee stands (C54, B086)
//   - 2026-11-04 23:50: names a nil target by its callee alone (C86 fix round 3)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
func Named(
	target starlark.Callable,
	callee string,
) string {
	if callee != "" || target == nil || target.Name() == LAMBDA {
		return callee
	}

	return target.Name()
}

// Attempt marks the evaluation as attempt n of a repeat or a retry, on the
// caller's own lane: an attempt is the caller's step, not a thread of its own.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-07 19:56: returns a common.BesideOption (C91w5, Q119)
//   - 2026-11-10 01:00: now uses ThreadID, were thread (C120a, Q314)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func Attempt(n int32) common.ForkOption {
	return func(
		parent common.Evaluation,
		child common.Evaluation,
	) {
		from, to, ok := _Pair(
			parent,
			child,
		)
		if !ok || from == nil || to == nil {
			return
		}

		to.attempt = n
		to.ThreadID = from.ThreadID
	}
}

// Inline keeps the evaluation on the caller's lane without counting it as an
// attempt, which is what a timeout wants: one call, bounded, not numbered.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-07 19:56: returns a common.BesideOption (C91w5, Q119)
//   - 2026-11-10 01:00: now uses ThreadID, were thread (C120a, Q314)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func Inline() common.ForkOption {
	return func(
		parent common.Evaluation,
		child common.Evaluation,
	) {
		from, to, ok := _Pair(
			parent,
			child,
		)
		if !ok || from == nil || to == nil {
			return
		}

		to.ThreadID = from.ThreadID
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
//   - 2026-11-07 19:56: returns a common.BesideOption (C91w5, Q119)
//   - 2026-11-10 01:00: now uses Catching, were catching (C120a, Q314)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func Catching() common.ForkOption {
	return func(
		parent common.Evaluation,
		child common.Evaluation,
	) {
		_, to, ok := _Pair(
			parent,
			child,
		)
		if !ok || to == nil {
			return
		}

		to.Catching++
	}
}

// Work runs the call on its own thread, watching the evaluation's context,
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
//   - 2026-11-02 00:48: stores the result _Evaluate fills on the handle
//   - 2026-11-02 12:05: returns the raw error, for the report, rather than
//     keeping it on the handle, so a kept handle is smaller
//   - 2026-11-02 12:05: arms the watch it is handed, so the watch needs no
//     object of its own
//   - 2026-11-04 00:30: stores the result through _Keep (C54, B079)
//   - 2026-11-06 00:02: returns ErrNoRun for a missing thread, locals or watch, where it
//     dereferenced one (C100a, B161, B162)
//   - 2026-11-08 21:04: a nil receiver is ErrNoRun (C112 fix round 2, B230, Q65)
//   - 2026-11-10 00:25: renamed from _Work, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun; uses _Eval, formerly _Evaluate (C121,
//     Q349, Q350)
func (h *Handle) Work(
	thread *starlark.Thread,
	locals *Locals,
	target starlark.Callable,
	watching *Watch,
) error {
	if h == nil || thread == nil || locals == nil || watching == nil {
		return ErrRun
	}

	watching.Arm(
		locals.Ctx,
		thread,
	)
	defer watching.Release()

	ended := new(_Result)

	_Eval(
		thread,
		locals,
		target,
		ended,
	)

	h.Keep(
		ended.value,
		ended.err,
	)

	return ended.raw
}

// _Result is what an evaluation produced: the value, the error as its caller
// reads it, and the error as the report reads it.
//
// Revisions:
//   - 2026-11-02 00:48: initial creation, so Within needs no Handle to carry a
//     result
type _Result struct {
	value starlark.Value
	err   error
	raw   error
}

// _Eval calls target on thread as the evaluation locals describes,
// guarded, and records on ended what it produced.
//
// The caller owns ended, so it stays on the caller's stack when the caller
// keeps it there. Returning a _Result by pointer would move it to the heap.
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
//   - 2026-11-02 00:48: fills a _Result the caller owns in place of a Handle
//   - 2026-11-02 12:28: an error that wraps trail.ErrFatal ends the run (C26)
//   - 2026-11-04 06:39: the error carries the place it arose, before anything
//     reads it, so a join, a record and the report all say the same (C69)
//   - 2026-11-10 01:00: now uses Locals, Fail, Run, Ctx, were _Locals, _Fail, run, ctx
//     (C120a, Q314)
//   - 2026-11-10 23:25: renamed from _Evaluate (C121, Q350)
func _Eval(
	thread *starlark.Thread,
	locals *Locals,
	target starlark.Callable,
	ended *_Result,
) {
	var called bool

	guard.WithRecover(
		&ended.value,
		&ended.err,
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

	ended.err = _Locate(
		ended.err,
		nil,
	)

	// A failure Fail kept is what this evaluation ended with, whether it went
	// on to return nothing - a loop over a file cut short - or to raise the
	// cancellation the run's stop then sent it. One the evaluation raised
	// itself, as assert does, is already what it returned.
	kept := locals.failed != nil && !errors.Is(
		ended.err,
		locals.failed,
	)
	if kept {
		ended.value = nil
		ended.err = locals.failed
	}

	ended.raw = ended.err

	// An error that wraps trail.ErrFatal - a value nested too deep - ends the
	// run as assert does: the one place that is asked, so no builtin calls
	// Fail for it. Catching is honoured, so retry catches it.
	if errors.Is(
		ended.err,
		trail.ErrFatal,
	) {
		_ = locals.Fail(ended.err)
	}

	// A panic is always a bug. End the run so the error reaches the caller
	// whether or not anybody joins this handle.
	if !called && ended.raw != nil {
		locals.Run._End(ended.raw)
	}

	if ended.err != nil && locals.Ctx.Err() != nil {
		ended.err = fmt.Errorf(
			"%w: %w",
			ErrCancelled,
			locals.Ctx.Err(),
		)
	}
}

// Report tells the run's reporter how this lane ended.
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
//   - 2026-11-02 12:05: takes the raw error, the handle no longer keeping it
//   - 2026-11-08 19:23: reads the thread and name through their methods; a nil receiver or run
//     reports nothing (C112, Q207, B230)
//   - 2026-11-10 00:25: renamed from _Report, as a test in tests/ needs it (C120a, Q314)
func (h *Handle) Report(
	run *Run,
	raw error,
) {
	if h == nil || run == nil {
		return
	}

	var (
		ended   = h.err
		outcome = run.Outcome()
	)

	if outcome != nil && raw != nil && errors.Is(
		raw,
		outcome,
	) {
		ended = raw
	}

	run._Tell(func() {
		run.into.Ended(
			h.Thread(),
			h.Name(),
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
//   - 2026-11-04 23:50: a nil or zero handle is over, so it answers nothing at once
//     (C86 fix round 3)
//   - 2026-11-07 19:56: takes a common.Handle; a nil one is over, as a nil *Handle was (C91w5,
//     Q127)
//   - 2026-11-10 01:00: now uses Of, Ctx, were _Of, ctx (C120a, Q314)
func Wait(
	thread *starlark.Thread,
	handle common.Handle,
) (starlark.Value, error) {
	locals, err := Of(thread)
	if err != nil {
		return nil, err
	}

	if handle == nil {
		handle = (*Handle)(nil)
	}

	select {
	case <-handle.Done():
		value, _, err := handle.Result()

		return value, err

	case <-locals.Ctx.Done():
		return nil, fmt.Errorf(
			"%w: %w",
			ErrCancelled,
			locals.Ctx.Err(),
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
// A nil handle is over already, so nothing is waited for.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation, as _Settle for join's abort
//   - 2026-09-21 09:46: exported, since join lives in a plugin now
//   - 2026-11-04 00:30: a nil handle returns at once (C54, B100)
//   - 2026-11-04 23:50: a nil handle is over, so there is nothing to wait for (C86 fix round 3)
//   - 2026-11-07 19:56: takes a common.Handle (C91w5, Q127)
//   - 2026-11-10 01:00: now uses Of, Ctx, were _Of, ctx (C120a, Q314)
func Settle(
	thread *starlark.Thread,
	handle common.Handle,
) {
	if handle == nil {
		return
	}

	locals, err := Of(thread)
	if err != nil {
		<-handle.Done()

		return
	}

	select {
	case <-handle.Done():
	case <-locals.Ctx.Done():
	}
}
