package observe

import (
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// Watcher is told every time a function changes status, while the run is
// going, so that a host decides for itself what is worth keeping and for how
// long.
//
// whole assembles the run as it stands, and is handed over as a function
// rather than a value because it is the expensive half: building it copies
// every node of every thread, so a run that is wide pays for it. A host
// logging changes never calls it; a host drawing a user interface calls it
// when something is actually watching. Measured at ten times the cost of a
// run when called on every change, and at nothing when not.
//
// Called on the goroutine of the thread that changed status, which makes two
// things the host's to handle. A Watcher that blocks holds up the script that
// triggered it. And a run with more than one thread calls this from more than
// one goroutine at once, so a Watcher that keeps anything needs a lock of its
// own.
type Watcher interface {
	Changed(change *workflowpb.Change, whole func() *workflowpb.Workflow)
}

// Watching returns the reporter that tells into about every change, built on
// the same recorder a status is assembled from.
//
// The same recorder rather than a fold of its own: what a watcher is handed
// has to be what a snapshot would have said, and two folds of one event
// stream are two things to keep in step.
//
// Revisions:
//   - 2026-09-23 22:48: initial creation
func Watching(into Watcher, opts ...Option) scheduler.Reporter {
	recorder := _NewRecorder()

	for _, opt := range opts {
		opt(recorder)
	}

	recorder.watch = into

	return recorder
}

// WithWatcher tells a run what to report every change to.
//
// Revisions:
//   - 2026-09-23 22:48: initial creation
func WithWatcher(into Watcher) Option {
	return func(recorder *_Recorder) {
		recorder.watch = into
	}
}

// _Changed is the change a node just became, copied.
//
// Copied because the recorder goes on writing to that node - a later attempt
// of a retry is the same node reaching RUNNING again - and a watcher handed
// the node itself would see a change it was never told about. The same
// aliasing a snapshot avoids, for the same reason.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-23 22:48: initial creation
func _Changed(thread string, node *workflowpb.Node) *workflowpb.Change {
	return &workflowpb.Change{
		Thread: thread,
		Node: &workflowpb.Node{
			Function: node.GetFunction(),
			Status:   node.GetStatus(),
			Attempt:  node.GetAttempt(),
			Failure:  node.GetFailure(),
		},
	}
}

// _Notify tells the watcher, if there is one.
//
// Called with the lock released, because a watcher may ask for the whole run
// and assembling that takes the same lock.
//
// Revisions:
//   - 2026-09-23 22:48: initial creation
func (r *_Recorder) _Notify(change *workflowpb.Change) {
	if r.watch == nil {
		return
	}

	r.watch.Changed(change, r._Whole)
}

// _Whole is the run as it stands, for a watcher that asks.
//
// Always running: this is only reachable while a run is producing changes, and
// how a run ended is the caller's to say once it has ended.
//
// Revisions:
//   - 2026-09-23 22:48: initial creation
func (r *_Recorder) _Whole() *workflowpb.Workflow {
	return &workflowpb.Workflow{
		Status:  workflowpb.Status_STATUS_RUNNING,
		Threads: r._Threads(),
	}
}
