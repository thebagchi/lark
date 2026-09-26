package observe

import (
	"context"
	"errors"
	"sync"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// Execution is one run: how to stop it, how to wait for it, and how it is
// doing.
//
// A caller holds this rather than an id, and nothing holds it for them. What
// is worth keeping about a finished run, and for how long, is the host's
// decision - so there is no store here, no identifier to look one up by, and
// nothing that forgets on a schedule of its own.
//
// It holds no context and no run. That is the point of it rather than an
// accident of what it happens to need: a cancel function is not a context - it
// cannot be read, derived from, or passed into an evaluation - so keeping one
// here says nothing about where a run's own context lives.
//
// value and err are written by the run's goroutine before it closes done, so
// done is the happens-before edge that makes both safe to read. Nothing reads
// either without having seen done closed.
type Execution struct {
	stop  context.CancelFunc
	done  chan struct{}
	value starlark.Value
	err   error
	into  *_Recorder
	once  sync.Once
}

// Start evaluates art's entry point on a goroutine of its own and returns at
// once with the run itself.
//
// The context passed in is the parent of the run's, so a caller that already
// has one keeps the ability to stop everything without this package needing to
// hold a context. Stop is the same stop by another route.
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
func Start(ctx context.Context, art *artifact.Artifact, opts ...Option) *Execution {
	into := _NewRecorder()

	for _, opt := range opts {
		opt(into)
	}

	inner, stop := context.WithCancel(scheduler.WithReporter(ctx, into))

	run := &Execution{
		stop: stop,
		done: make(chan struct{}),
		into: into,
	}

	go func() {
		defer close(run.done)
		defer stop()

		run._Perform(inner, art)
	}()

	return run
}

// Stop ends the run. It does not wait, and calling it twice is calling it
// once.
//
// The interpreter is told between instructions, so a script with no sleep in
// it stops as readily as one that blocks. What Wait then returns is
// ErrCancelled, which is the same answer a run stopped through its context
// gives, because it is the same stop.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation, replacing Store.Cancel
func (e *Execution) Stop() {
	e.once.Do(e.stop)
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

// Status is how this run is doing, or how it ended.
//
// The run's status is the run's own, never folded from its functions': a run
// that succeeded without reaching everything its graph declared is succeeded,
// and the functions it did not reach stay pending. Folding would make such a
// run report itself pending for ever, since a pending node has nothing further
// to happen to it.
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
func (e *Execution) Status() *workflowpb.Workflow {
	if !e._Over() {
		return &workflowpb.Workflow{
			Status:  workflowpb.Status_STATUS_RUNNING,
			Threads: e.into._Threads(),
		}
	}

	status, failure := _Became(e.err), _Why(e.err)

	return &workflowpb.Workflow{
		Status:  status,
		Threads: e.into._Threads(),
		Cause:   e.into._Because(status, failure),
	}
}

// _Over reports whether this run has finished.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (e *Execution) _Over() bool {
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}

// _Perform evaluates art with the run's own file open around it, and records
// how it ended.
//
// Guarded, because this is a goroutine of ours and a panic on it cannot be
// recovered from outside - it would take the host down rather than fail the
// run. Invoke guards the script's own evaluation; this guards everything
// before it gets there, which is where an artifact that is not one lands.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation, lifting the goroutine's body so the
//     run's file has somewhere to be opened and closed
//   - 2026-09-23 23:20: a method on the run it performs
func (e *Execution) _Perform(ctx context.Context, art *artifact.Artifact) {
	err := e.into._Open()
	if err != nil {
		e.err = err

		return
	}

	guard.WithRecover(
		&e.value,
		&e.err,
		func() (starlark.Value, error) {
			return art.Run(ctx)
		},
	)

	// After the evaluation, which waited for every thread it started, so
	// nothing is still printing.
	closing := e.into._Close()
	if e.err == nil {
		e.err = closing
	}
}

// _Became is the status an error ends something with.
//
// A cancellation is not a failure. A run a caller stopped did not break, and a
// thread stopped because something else failed did not itself fail - reporting
// either as failed says something went wrong where nothing did.
//
// The run's own outcome still wins, and that is what makes this safe: Invoke
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
