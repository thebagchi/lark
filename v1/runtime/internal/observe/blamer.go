package observe

import (
	"errors"
	"sync"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// _Blamer is the failure to blame for a run, which both books keep: the cause
// Cause names, and the error behind it, kept so _Blame can tell a caller that
// propagated an inner failure from a caller that failed on its own.
//
// It has a lock of its own, which only a failed call takes, so a ledger's
// threads wait for each other on a failure and on nothing else. The recorder
// takes it under its own lock, and nothing takes the two the other way.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation, lifted from _Recorder so the ledger blames by
//     the same rule
type _Blamer struct {
	mu     sync.Mutex
	cause  *workflowpb.Cause
	blamed error
}

// Because is the cause to report for a run that ended this way, or nil.
//
// Nil unless the run failed, so a host tests the pointer rather than the enum.
//
// A run can fail with no call blamed - a failure the scheduler saw and no
// thread did. Then the run's own text stands in, with no function named:
// something went wrong and this is all that is known, which is more use than a
// nil a host reads as "nothing failed".
//
// Revisions:
//   - 2026-09-20 11:39: initial creation, as _Recorder._Because
//   - 2026-11-02 17:49: moved to _Blamer, which both books share
//   - 2026-11-10 00:25: renamed from _Because, as a test in tests/ needs it (C120a, Q314)
func (b *_Blamer) Because(
	status workflowpb.Status,
	failure string,
) *workflowpb.Cause {
	if status != workflowpb.Status_STATUS_FAILED {
		return nil
	}

	found := b.Cause()
	if found != nil {
		return found
	}

	return &workflowpb.Cause{
		Failure: failure,
	}
}

// Cause is the call whose failure ended the run, or nil.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation, as _Recorder._Cause
//   - 2026-11-02 17:49: moved to _Blamer, which both books share
//   - 2026-11-10 00:25: renamed from _Cause, as a test in tests/ needs it (C120a, Q314)
func (b *_Blamer) Cause() *workflowpb.Cause {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.cause
}

// _Blame remembers which call to blame for a failure, so Cause names the
// function whose error Wait returns.
//
// It overwrites an earlier blame when a later failure arrives, unless the
// two errors share a chain. The check is bidirectional: the new error may
// wrap the old, or the old may wrap the new, because Starlark and the
// scheduler can wrap the same cause in different directions. A shared chain
// means one error propagated the other, not a new failure.
//
// join picks the first failure in argument order, and it waits for each
// handle in turn. A function that failed before the chosen one in wall
// time but after it in argument order is overwritten when the chosen one
// arrives. A caller that propagated the chosen one is not.
//
// A thread stopped because something else failed reports a cancellation,
// not a failure, so only a real failure lands here, and any other status
// returns before the lock.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation, as _Recorder._Blame
//   - 2026-09-21 01:21: takes the node's identity, which the key holds, so the
//     message needs no thread of its own
//   - 2026-09-21 08:09: reached through a pointer, as every struct here is
//   - 2026-10-02 01:31: takes the thread and the line's position, which the
//     cause carries as its index
//   - 2026-10-02 15:34: takes the function, its status and its error, a cause
//     naming the function alone
//   - 2026-10-24 20:30: overwrite unless the errors share a chain, so Cause
//     names the function whose error Wait returns
//   - 2026-11-02 17:49: moved to _Blamer, which both books share, and takes its own
//     lock
//   - 2026-11-10 01:00: now uses Why, were _Why (C120a, Q314)
func (b *_Blamer) _Blame(
	name string,
	status workflowpb.Status,
	err error,
) {
	if status != workflowpb.Status_STATUS_FAILED {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	propagated := b.blamed != nil && (errors.Is(
		err,
		b.blamed,
	) || errors.Is(
		b.blamed,
		err,
	))
	if propagated {
		return
	}

	b.cause = &workflowpb.Cause{
		Function: name,
		Failure:  Why(err),
	}
	b.blamed = err
}
