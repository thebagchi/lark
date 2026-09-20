package observe

import (
	"context"
	"errors"
	"time"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// _Entry is one run the store holds: how to stop it, how to tell when it is
// over, and what it produced.
//
// It holds no context and no run. That is the point of it rather than an
// accident of what it happens to need: review.md's phase 3 defends a context
// living on a _Run by arguing that a _Run is unreachable once the call that
// made it returns, and warns that anything storing one past that call silently
// breaks the argument. A cancel function is not a context - it cannot be read,
// derived from, or passed into an evaluation - so keeping one here says
// nothing about where a run's own context lives.
//
// value, err and ended are written by the run's goroutine before it closes
// done, so done is the happens-before edge that makes all three safe to read.
// Nothing reads any of them without having seen done closed.
type _Entry struct {
	id    string
	stop  context.CancelFunc
	done  chan struct{}
	value starlark.Value
	err   error
	ended time.Time
	into  *_Recorder
}

// _Over reports whether this run has finished.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (e *_Entry) _Over() bool {
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}

// _Ending is how a finished run ended.
//
// Asked only of a finished run. done is what makes err safe to read, so a
// caller establishes that first - which _Over is for.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
//   - 2026-09-20 11:34: tells a cancellation from a failure, which the schema
//     had no value for until now
//   - 2026-09-20 11:36: returns the text as well, so a host has something to
//     show beside a red box
func (e *_Entry) _Ending() (workflowpb.Status, string) {
	return _Became(e.err), _Why(e.err)
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
// A cancellation's own text reads "handle cancelled: context canceled", which
// says what the status says, in more words. Leaving it empty is what lets a
// host treat a non-empty failure as "there is something to show" without first
// looking at the status.
//
// Revisions:
//   - 2026-09-20 11:36: initial creation
func _Why(err error) string {
	if _Became(err) != workflowpb.Status_STATUS_FAILED {
		return ""
	}

	return err.Error()
}
