package scheduler

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// ErrNested is returned when an evaluation inside an update, or anything that
// evaluation started, begins another.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 20:10: an alias of run.ErrNested (C91w5)
var (
	ErrNested = run.ErrNested
)

// Release frees the hold h stands for, and does nothing when that hold is
// already freed.
//
// It is safe to call twice, from many goroutines together, and after the
// evaluation has taken a later hold: a call frees only the hold it came from,
// so a stale Hold cannot free a newer one and break the lock. On a zero Hold,
// or one whose Owner the scheduler did not make, it does nothing.
//
// Revisions:
//   - 2026-11-05 22:40: initial creation (C58 fix 2, Q201, Q85, B124)
//   - 2026-11-07 20:10: a function of common.Hold, since Hold moved to common as data (C91w5,
//     Q229)
func Release(h common.Hold) {
	run.Release(h)
}

// Lock takes the run's lock for name on behalf of the evaluation on thread, and
// returns the Hold that releases it.
//
// It waits on the evaluation's context as well as the lock, which a
// sync.Mutex cannot: a run cancelled while a thread waits for a name another
// thread holds must end, not hang.
//
// The Hold frees its own hold only: released again, or after a later Lock, it
// does nothing.
//
// Returns ErrNested when the evaluation, or the evaluation that started it, is
// already inside an update - so a nested update is refused through a spawn or
// a timeout as surely as on the thread that started it. Two threads updating
// two names in opposite orders would deadlock, and a script cannot be asked to
// take locks in an order it cannot see. Returns ErrCancelled wrapping the
// context's error when the wait is cancelled.
//
// **A thread spawned inside an update cannot lock for the rest of its life.**
// It copied the fact at birth and nothing clears a copy, so it is refused
// after the update has returned as surely as during it. That is deliberate
// rather than an oversight: clearing the mark on release would make the same
// script succeed or fail on timing, since the child's set would land either
// side of a release it cannot see. A ban a reader can predict beats a failure
// that flickers. A thread that needs to lock is spawned before the update, not
// inside it.
//
// The two refusals say different things because they are different facts. An
// evaluation that took the name is already updating. One that inherited the
// mark was started while another evaluation was, and saying "already updating"
// of it is false once that other evaluation has finished.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-09-21 09:46: the name held is one string, since there is never a
//     second
//   - 2026-10-28 22:06: takes a free slot without a wait on the context, and
//     hands back the release the evaluation keeps rather than a new closure
//   - 2026-10-29 01:05: finds the evaluation through EvaluationOf, and keeps
//     its body in the Lock of Evaluation
//   - 2026-11-05 22:05: the release frees its own hold only, so a repeated or stale call does
//     nothing (C58 fix round 1, Q85)
//   - 2026-11-05 22:40: returns a Hold, a value with no allocation (C58 fix 2, Q201)
//   - 2026-11-07 20:10: forwards to run.Lock (C91w5)
func Lock(
	thread *starlark.Thread,
	name string,
) (common.Hold, error) {
	return run.Lock(
		thread,
		name,
	)
}

// Updating is the name the evaluation on thread is inside an update of, and
// empty when it is inside none.
//
// Exported for a builtin that must refuse while a name is held without
// wanting the lock itself. join is the one: waiting for a thread that needs
// the name this evaluation holds is a wait nothing in the script can end, and
// only the scheduler knows a name is held.
//
// True for an evaluation that inherited the mark as well as one that took it,
// because both are inside an update as far as anything they do is concerned.
//
// Revisions:
//   - 2026-09-24 17:12: initial creation
//   - 2026-11-07 20:10: forwards to run.Updating (C91w5)
func Updating(thread *starlark.Thread) (string, error) {
	return run.Updating(thread)
}
