// Package observe starts runs that outlive the call starting them, and keeps
// the graph of what each one is doing while it does it.
//
// A host starts a script and is handed the run itself, which it stops, waits
// for and asks about for as long as it holds it. Nothing here blocks the
// caller and nothing here keeps a run alive - a run is a goroutine evaluating
// an artifact.
//
// Nothing here keeps a finished run either. What is worth remembering about
// one, and for how long, is the host's decision and not this runtime's, so
// there is no store, no identifier to look a run up by, and nothing that
// forgets on a schedule of its own. A host that wants a run after it has let
// go of it keeps its last Status.
package observe

import (
	"context"
	"errors"
	"slices"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/guard"
)

// Execution is one run: how to wait for it, and how it is doing. Stopping it is
// cancelling the context it was started with.
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
type Execution struct {
	done  chan struct{}
	value starlark.Value
	err   error
	into  *_Recorder
}

// Start evaluates art's entry point on a goroutine of its own and returns at
// once with the run itself.
//
// The context passed in is the parent of the run's, and cancelling it is the
// one way to stop the run. The interpreter is told between instructions, so a
// script with no sleep in it stops as readily as one that blocks, and Wait
// then returns ERR_CANCELLED.
//
// Its graph is this package's own, kept for Status and changed step by step,
// each change going where WithChanges says.
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
func Start(ctx context.Context, art *artifact.Artifact, opts ...artifact.RunOption) *Execution {
	into := _NewRecorder()
	into.watch = artifact.Changes(opts)
	into._Began()

	inner, stop := context.WithCancel(ctx)

	// Last, so nothing in opts replaces the recorder Status reads, and copied,
	// so the caller's slice is not written into.
	given := slices.Concat(opts, []artifact.RunOption{artifact.Reporting(into)})

	run := &Execution{
		done: make(chan struct{}),
		into: into,
	}

	go func() {
		defer close(run.done)
		defer stop()

		run._Perform(inner, art, given)
	}()

	return run
}

// Wait blocks until the run is over and returns what it produced.
//
// Every caller that waits is told, and told the same thing, however many of
// them there are and whenever they ask. A caller that would rather not block
// waits on Done instead.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation, as Store.Wait
//   - 2026-09-23 23:20: waits on the run it is a method of, so there is no id
//     to be unknown and no context needed to give up with - Done is that
func (e *Execution) Wait() (starlark.Value, error) {
	<-e.done

	return e.value, e.err
}

// Done is closed when the run is over, for a caller selecting on it beside
// something of its own.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func (e *Execution) Done() <-chan struct{} {
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
func (e *Execution) Status() *workflowpb.Graph {
	return e.into._Drawn()
}

// _Perform evaluates art with the options opts, and records how it ended, in
// the run and in its graph.
//
// Guarded, because this is a goroutine of ours and a panic on it cannot be
// recovered from outside - it would take the host down rather than fail the
// run. The run guards the script's own evaluation; this guards everything
// before it gets there, which is where an artifact that is not one lands.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation, lifting the goroutine's body so the
//     run's file has somewhere to be opened and closed
//   - 2026-09-23 23:20: a method on the run it performs
//   - 2026-10-02 13:12: opens no file, a run's transcript being its context's
//   - 2026-10-02 16:08: runs art with the run's options
//   - 2026-10-02 16:33: tells the graph how the run ended, which is its last
//     change
func (e *Execution) _Perform(
	ctx context.Context,
	art *artifact.Artifact,
	opts []artifact.RunOption,
) {
	guard.WithRecover(
		&e.value,
		&e.err,
		func() (starlark.Value, error) {
			return artifact.Run(ctx, art, opts...)
		},
	)

	status := _Became(e.err)

	e.into._Finished(status, e.into._Because(status, _Why(e.err)))
}

// _Became is the status an error ends something with.
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
func _Became(err error) workflowpb.Status {
	if err == nil {
		return workflowpb.Status_STATUS_SUCCEEDED
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return workflowpb.Status_STATUS_CANCELLED
	}

	return workflowpb.Status_STATUS_FAILED
}

// _Why is the text to show for an error, or empty when the status already says
// everything.
//
// A cancellation's own text says what the status says, in more words. Leaving
// it empty is what lets a host treat a non-empty failure as "there is
// something to show" without first looking at the status.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
func _Why(err error) string {
	if _Became(err) != workflowpb.Status_STATUS_FAILED {
		return ""
	}

	return err.Error()
}
