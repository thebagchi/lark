// Package observe is internal, not public: it holds the runs that outlive
// their caller, and the graph of each.
//
// Package observe starts runs that outlive the call starting them, and keeps
// the graph of what each one is doing while it does it.
//
// A host starts a script and is handed the run itself, which it stops, waits
// for and asks about for as long as it holds it. Nothing here keeps a run
// alive - a started run is a goroutine evaluating an artifact. A host that
// would only wait evaluates instead: the run is then the host's own goroutine,
// and what it is handed at the end is its value, its graph and its prints.
//
// Nothing here keeps a finished run either. What is worth remembering about
// one, and for how long, is the host's decision and not this runtime's, so
// there is no store, no identifier to look a run up by, and nothing that
// forgets on a schedule of its own. A host that wants a run after it has let
// go of it keeps its last Status.
//
// Revisions:
//   - 2026-09-23 22:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:23: moved under v1/runtime/internal (C91w7, Q108)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
package observe

import (
	"context"
	"errors"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

// Execution is the run that implements common.Execution: how to wait for it, and
// how it is doing. Stopping it is cancelling the context it was started with.
//
// A caller holds this rather than an id, and nothing holds it for them. What
// is worth keeping about a finished run, and for how long, is the host's
// decision - so there is no store here, no identifier to look one up by, and
// nothing that forgets on a schedule of its own.
//
// It holds no context and no run: the context is the caller's, and the run is
// a goroutine.
//
// value and err are written by the run's goroutine before it closes done, so
// done is the happens-before edge that makes both safe to read. Nothing reads
// either without having seen done closed.
//
// Revisions:
//   - 2026-09-20 19:57: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:23: implements common.Execution, which now holds the
//     interface (C91w7, Q101, Q117)
//   - 2026-11-10 00:25: renamed the fields err, as a test in tests/ needs them (C120a, Q314)
type Execution struct {
	done  chan struct{}
	value starlark.Value
	Err   error
	into  _Book
}

// Start evaluates art's entry point on a goroutine of its own and returns at
// once with the run itself.
//
// The context passed in is the parent of the run's, and cancelling it is the
// one way to stop the run. The interpreter is told between instructions, so a
// script with no sleep in it stops as readily as one that blocks, and Wait
// then returns scheduler.ErrCancelled.
//
// Its graph is this package's own. With WithTracking, or WithChanges, it is
// kept for Status and changed step by step, each change going where WithChanges
// says. With neither, Status answers an empty running graph until the run
// ends, and the graph of the whole run after it.
//
// The error is returned when Start cannot begin the run: artifact.ErrArtifact for an
// artifact lark did not compile, a nil one included. The run is then nil and
// must not be used. A run that fails after it began still returns the run, and
// Wait reports the fault.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as Store.Start, returning an id
//   - 2026-09-20 01:41: takes options, builds the run a recorder, and puts that
//     recorder where the scheduler will find it
//   - 2026-09-20 11:56: guards the goroutine, so a panic before the script is
//     reached fails the run instead of the process
//   - 2026-09-21 16:42: the goroutine's body opens and closes the run's own
//     file around the evaluation
//   - 2026-09-23 23:20: returns the run rather than an id, and no store holds
//     it - what a finished run is worth keeping is the host's to decide
//   - 2026-10-02 13:12: takes no options, since what a run prints is its
//     context's transcript and nothing else is chosen here
//   - 2026-10-02 16:08: takes the run's options, which it hands to the run
//   - 2026-10-02 16:33: sends the graph's changes where the options say, from
//     the first, which makes an empty graph running
//   - 2026-10-03 00:20: keeps no cancel function, the caller's context being the one
//     way to stop a run
//   - 2026-10-03 08:31: hands the run its recorder as an option, rather than on a
//     context
//   - 2026-10-24 16:10: contain a panic in the change callback at _Began, so
//     it fails the run instead of the process
//   - 2026-10-28 23:50: opens and performs the run through _Opened and
//     _Perform, which Evaluate calls too, so the two cannot drift
//   - 2026-10-29 01:26: applies opts once, and hands what they told to both
//   - 2026-11-02 17:49: keeps the live graph only when the options ask for it
//   - 2026-11-07 17:40: also returns an error, always nil for now (C91 wave 0)
//   - 2026-11-07 23:04: takes common.Artifact and common.RunOption, and returns ErrNotCompiled
//     with a nil run for a nil or foreign artifact (C91w6, Q118, Q125)
//   - 2026-11-07 23:23: returns common.Execution, nil with an error (C91w7, Q101, Q233)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Err, were err (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrArtifact, formerly ErrNotCompiled; uses WithTracking, formerly
//     WithLiveGraph (C121, Q348, Q350)
func Start(
	ctx context.Context,
	art common.Artifact,
	opts ...common.RunOption,
) (common.Execution, error) {
	held, err := artifact.Concrete(art)
	if err != nil {
		return nil, err
	}

	var (
		given       = artifact.Apply(opts)
		into, fault = _Opened(given)
	)
	if fault != nil {
		run := &Execution{
			done: make(chan struct{}),
			into: into,
			Err:  fault,
		}
		close(run.done)

		return run, nil
	}

	run := &Execution{
		done: make(chan struct{}),
		into: into,
	}

	go func() {
		defer close(run.done)

		run.value, run.Err = _Perform(
			ctx,
			held,
			into,
			given,
		)
	}()

	return run, nil
}

// Wait blocks until the run is over and returns what it produced.
//
// A zero or nil Execution, one Start did not return, returns a plain error at
// once: no host can hold one, so the error has no name to match.
//
// Every caller that waits is told, and told the same thing, however many of
// them there are and whenever they ask. A caller that would rather not block
// waits on Done instead.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation, as Store.Wait
//   - 2026-09-23 23:20: waits on the run it is a method of, so there is no id
//     to be unknown and no context needed to give up with - Done is that
//   - 2026-11-04 23:50: a zero Execution returns ErrNotStarted at once (C86)
//   - 2026-11-04 23:50: a nil *Execution does too (C86 fix round 2)
//   - 2026-11-08 13:25: the error is a plain one; ErrNotStarted is dropped (C98, Q249)
//   - 2026-11-10 01:00: now uses Err, were err (C120a, Q314)
func (e *Execution) Wait() (starlark.Value, error) {
	if e == nil || e.done == nil {
		return nil, errors.New("the run was not started")
	}

	<-e.done

	return e.value, e.Err
}

// Done is closed when the run is over, and at once for a zero Execution, for a
// caller selecting on it beside something of its own.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-11-04 23:50: a zero Execution's Done is closed (C86)
//   - 2026-11-04 23:50: so is a nil *Execution's (C86 fix round 2)
func (e *Execution) Done() <-chan struct{} {
	if e == nil || e.done == nil {
		closed := make(chan struct{})
		close(closed)

		return closed
	}

	return e.done
}

// Status is how this run is doing, or how it ended: every function it has
// called, with its status, the calls between them, and what ended the run when
// it failed.
//
// The run's status is the run's own, never folded from its functions': a run
// whose main returned succeeded, even while a thread nothing joined was still
// running when it ended and was cancelled.
//
// It is the graph the run's changes make, step for step, so it turns finished
// with the last change rather than when Done closes, a moment later. A caller
// told the run ended, and waiting for it then, is not kept waiting.
//
// Without WithTracking or WithChanges the graph is kept for the end only: asked
// before the run ends, Status answers a graph with the running status and
// nothing else.
//
// A zero or nil Execution, one Start did not return, has no run to ask: it answers an
// empty graph with no status and no functions, never nil.
//
// Asking does not consume. A run answers for as long as the caller holds it,
// which is now as long as the caller wants rather than as long as a sweep
// allows.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation, as Store.Status
//   - 2026-09-20 12:04: stops forgetting a run it reported, so two interfaces
//     watching one run are both answered
//   - 2026-09-23 23:20: a method on the run, which the caller holds, so there
//     is no id and nothing to have forgotten
//   - 2026-10-02 01:32: a Graph
//   - 2026-10-02 15:34: a call graph, of functions and the calls between them
//   - 2026-10-02 16:45: the graph the run's changes make, its status and cause
//     included, rather than one finished when Done closes
//   - 2026-11-02 17:49: answers an empty running graph before the end when the run
//     keeps no live graph
//   - 2026-11-04 23:50: a zero Execution answers an empty graph (C86)
//   - 2026-11-04 23:50: so does a nil *Execution (C86 fix round 2)
//   - 2026-11-10 01:00: now uses Drawn, were _Drawn (C120a, Q314)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func (e *Execution) Status() *workflowpb.Graph {
	if e == nil || e.into == nil {
		return new(workflowpb.Graph)
	}

	return e.into.Drawn()
}

// Snapshot is the graph as the run holds it now, with the status RUNNING, for a
// run that will not end by itself: every function called so far, with the
// calls between them, and the node of a call still going shown RUNNING.
//
// Unlike Status, it answers at any moment, whether or not the run keeps a live
// graph (WithTracking), and a run without one pays nothing for it until it is
// asked. It is not an instant of the run: threads keep running while it is
// drawn, so a call started meanwhile can show on one thread's account and not
// on another's. Every part of it was true at some moment.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-04 23:50: a zero Execution answers an empty graph (C86)
//   - 2026-11-04 23:50: so does a nil *Execution (C86 fix round 2)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func (e *Execution) Snapshot() *workflowpb.Graph {
	if e == nil || e.into == nil {
		return new(workflowpb.Graph)
	}

	return e.into._Standing()
}

// _Opened is the book of a run about to start with what given told it, its
// graph already running, and the fault of a watcher that panicked when told
// so.
//
// It is the one place that picks the book. A run that asked for the live graph
// (WithTracking), or that has a watcher to tell its changes to, gets the
// Recorder, which answers Status at any moment. Any other run gets the
// Ledger, which folds with no lock the threads share and answers Status with
// an empty running graph until the run ends.
//
// A fault here ends the run before it starts, which Start and Eval each
// report in their own way.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation, lifted from Start so Evaluate opens a
//     run the same way
//   - 2026-10-29 01:26: takes the options applied, rather than applying them
//     to find the watcher
//   - 2026-11-02 17:49: picks the recorder only for a run that asked for the live graph
//     or has a watcher, and the ledger for any other
//   - 2026-11-07 23:04: reads the fields of common.Given (C91w6)
//   - 2026-11-10 01:00: now uses Recorder, Ledger, NewRecorder, Watch, and 2 more, were _Recorder,
//     _Ledger, _NewRecorder, watch, and 2 more (C120a, Q314)
//   - 2026-11-10 22:40: uses Eval and WithTracking, formerly Evaluate and WithLiveGraph (C121,
//     Q350)
func _Opened(given *common.Given) (_Book, error) {
	var into _Book

	if given.Live || given.Watch != nil {
		recorder := NewRecorder()
		recorder.Watch = given.Watch
		into = recorder
	} else {
		into = NewLedger()
	}

	into.Began()

	return into, into._Fault()
}

// _Perform evaluates art with what given told it, reporting to into, and
// returns how it ended, which it has also recorded in into's graph.
//
// Guarded, because a panic on the goroutine Start runs it on cannot be
// recovered from outside - it would take the host down rather than fail the
// run - and one on a caller's goroutine is a host's crash too. The run guards
// the script's own evaluation; this guards everything before it gets there,
// which is where an artifact that is not one lands.
//
// It derives no context of its own. The run derives its context from ctx when
// it begins and cancels it when it ends, before it returns, so a second
// context here was one more to make and cancel with nothing that read it.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation, lifting the goroutine's body so the
//     run's file has somewhere to be opened and closed
//   - 2026-09-23 23:20: a method on the run it performs
//   - 2026-10-02 13:12: opens no file, a run's transcript being its context's
//   - 2026-10-02 16:08: runs art with the run's options
//   - 2026-10-02 16:33: tells the graph how the run ended, which is its last
//     change
//   - 2026-10-24 16:10: surface a watcher fault from _Finished as the run's
//     error, so a panic in the change callback fails the run
//   - 2026-10-28 23:50: a function of the recorder, returning the value and the
//     error, so Start and Evaluate perform a run in one way; derives and
//     cancels the run's context and adds the recorder to opts itself
//   - 2026-10-29 01:26: takes the options applied, adds the recorder to them in
//     place, and hands the run ctx itself, the run deriving its own context
//   - 2026-11-02 17:49: takes the run's book, whichever kind it is
//   - 2026-11-05 00:27: tells the book the spawns the stop may have prevented,
//     before the run's last change (C45, Q18)
//   - 2026-11-07 23:04: takes common.Given (C91w6)
//   - 2026-11-10 01:00: now uses Became, Unstarted, Written, Finished, and 2 more, were _Became,
//     _Unstarted, _Written, _Finished, and 2 more (C120a, Q314)
func _Perform(
	ctx context.Context,
	art *artifact.Artifact,
	into _Book,
	given *common.Given,
) (starlark.Value, error) {
	// Last, so nothing in the options replaces the recorder the graph is read
	// from. given is this run's own, so adding it there changes no other run.
	artifact.Reporting(into)(given)

	var (
		value starlark.Value
		err   error
	)

	guard.WithRecover(
		&value,
		&err,
		func() (starlark.Value, error) {
			return artifact.Perform(
				ctx,
				art,
				given,
			)
		},
	)

	status := Became(err)

	into.Unstarted(Written(
		art,
		status,
	))

	into.Finished(
		status,
		into.Because(
			status,
			Why(err),
		),
	)

	if fault := into._Fault(); fault != nil && err == nil {
		err = fault
	}

	return value, err
}

// Became is the status an error ends something with.
//
// A cancellation is not a failure. A run a caller stopped did not break, and a
// thread stopped because something else failed did not itself fail - reporting
// either as failed says something went wrong where nothing did.
//
// The run's own outcome still wins, and that is what makes this safe: a run
// returns a recorded outcome before it looks at the context, so a script that
// asserted and was then torn down reports the assertion. Only a failure with no
// outcome behind it reaches here carrying a context error.
//
// Revisions:
//   - 2026-09-20 11:34: initial creation
//   - 2026-11-10 00:25: renamed from _Became, as a test in tests/ needs it (C120a, Q314)
func Became(err error) workflowpb.Status {
	if err == nil {
		return workflowpb.Status_STATUS_SUCCEEDED
	}

	if errors.Is(
		err,
		context.Canceled,
	) || errors.Is(
		err,
		context.DeadlineExceeded,
	) {
		return workflowpb.Status_STATUS_CANCELLED
	}

	return workflowpb.Status_STATUS_FAILED
}

// Why is the text to show for an error, or empty when the status already says
// everything.
//
// A cancellation's own text says what the status says, in more words. Leaving
// it empty is what lets a host treat a non-empty failure as "there is
// something to show" without first looking at the status.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
//   - 2026-11-10 00:25: renamed from _Why, as a test in tests/ needs it (C120a, Q314)
func Why(err error) string {
	if Became(err) != workflowpb.Status_STATUS_FAILED {
		return ""
	}

	return err.Error()
}
