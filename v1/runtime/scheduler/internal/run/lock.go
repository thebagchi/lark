package run

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrNested is returned when an evaluation inside an update, or anything that
// evaluation started, begins another.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
var (
	ErrNested = errors.New("an update cannot start another")
)

// Release frees the hold h stands for, and does nothing when that hold is
// already freed.
//
// It is safe to call twice, from many goroutines together, and after the
// evaluation has taken a later hold: a call frees only the hold it came from,
// so a stale Hold cannot free a newer one and break the lock. On a zero Hold,
// or a Hold whose Owner this package did not make, it does nothing.
//
// Revisions:
//   - 2026-11-05 22:40: initial creation (C58 fix 2, Q201, Q85, B124)
//   - 2026-11-07 19:55: a function of common.Hold, since Hold moved to common as data; it asserts
//     the
//     owner to its concrete type (C91w5, Q229)
//   - 2026-11-10 01:00: now uses Locals, Release, were _Locals, _Release (C120a, Q314)
func Release(h common.Hold) {
	locals, ok := h.Owner.(*Locals)
	if !ok {
		return
	}

	locals.Release(h.Token)
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
//   - 2026-11-07 19:55: returns a common.Hold, and finds the locals itself, so the Evaluation
//     stays on the
//     stack (C91w5, Q229)
//   - 2026-11-10 01:00: now uses Of, were _Of (C120a, Q314)
func Lock(
	thread *starlark.Thread,
	name string,
) (common.Hold, error) {
	locals, err := Of(thread)
	if err != nil {
		return common.Hold{}, err
	}

	evaluation := &Evaluation{
		locals: locals,
	}

	return evaluation.Lock(name)
}

// _Slot returns the one-place channel that is the lock for name, making it the
// first time it is asked for.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-10-29 01:24: makes the lock map on the run's first lock
//   - 2026-11-10 01:00: now uses Run, were _Run (C120a, Q314)
func (r *Run) _Slot(name string) chan struct{} {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.locks == nil {
		r.locks = make(map[string]chan struct{})
	}

	slot, found := r.locks[name]
	if !found {
		slot = make(
			chan struct{},
			1,
		)
		r.locks[name] = slot
	}

	return slot
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
//   - 2026-11-10 01:00: now uses Of, were _Of (C120a, Q314)
func Updating(thread *starlark.Thread) (string, error) {
	locals, err := Of(thread)
	if err != nil {
		return "", err
	}

	return locals.inside, nil
}

// Take puts the one token in slot, and waits for room only when the slot is
// full.
//
// A free slot is the common case, and a select with a default costs much less
// than a select that also watches the context. So a free slot is taken even
// when the context is already done: the evaluation then stops at its next
// instruction, and its deferred release gives the slot back.
//
// Returns ErrCancelled wrapping the context's error when the wait is
// cancelled.
//
// Revisions:
//   - 2026-10-28 22:06: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-03 21:57: a full slot with a nil ctx is ErrNoRun, for a nil ctx is no
//     evaluation's and nothing could end the wait (C52, B074)
//   - 2026-11-10 00:25: renamed from _Take, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func Take(
	ctx context.Context,
	slot chan struct{},
	name string,
) error {
	select {
	case slot <- struct{}{}:
		return nil
	default:
	}

	if ctx == nil {
		return fmt.Errorf(
			"waiting for %s: %w",
			unpack.QuotePrefix(name),
			ErrRun,
		)
	}

	select {
	case slot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf(
			"waiting for %s: %w: %w",
			unpack.QuotePrefix(name),
			ErrCancelled,
			ctx.Err(),
		)
	}
}

// Release gives back the slot of the hold numbered token, and clears the mark.
// It does nothing when that hold is no longer the one held, so a second call, a
// call that comes after a later Lock, and many calls together free the hold
// once.
//
// Each Hold carries its hold's number, so one kept past its use cannot free a
// hold taken after it, and it costs no allocation. The number is claimed with a
// compare and swap, so only one caller clears the fields. An evaluation holds
// at most one name: a second lock is refused while it holds one.
//
// The winner swaps the token for RELEASING, clears the fields, and only then
// stores 0. A call that loses the claim spins, giving up its turn with
// runtime.Gosched so that a winner on the same P can run, until HoldNumber is
// no longer RELEASING. So when any call returns, the hold is cleared and no write
// of its fields is still to come: the owner can Lock again at once. A loser can
// return before the winner has drained the slot; the owner's next Lock then
// waits for the slot in Take.
//
// Memory ordering: Go's atomics are sequentially consistent, and a load that
// sees a value happens after the store of that value. The store of 0 comes after
// the writes to holding, inside and slot, so a loser that loads anything but
// RELEASING sees those writes, and so does the owner that Locks after it. The
// swap to RELEASING comes before the writes, so no loser leaves before the
// winner has the claim. The owner's next Lock writes the fields and then stores
// its token, so a later Release sees them in the same way. A token equal to
// RELEASING is refused, so no caller can claim a hold that is being freed.
//
// Revisions:
//   - 2026-10-28 22:06: initial creation
//   - 2026-11-05 19:07: a call with no slot held returns, where it waited for ever (C58, B124)
//   - 2026-11-05 22:05: acts only on the hold its token names (C58 fix round 1, Q85)
//   - 2026-11-05 22:40: called by Hold.Release (C58 fix 2)
//   - 2026-11-06 00:02: does nothing for a nil receiver, for token 0, which no hold carries, or
//     for a hold with no slot (C100a, B165)
//   - 2026-11-10 00:25: renamed from _Release, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 12:15: a call that loses the claim spins on the sentinel RELEASING until the
//     winner has cleared the hold (C116, B236, Q329)
func (l *Locals) Release(token uint64) {
	if l == nil || token == 0 || token == RELEASING {
		return
	}

	if !l.HoldNumber.CompareAndSwap(
		token,
		RELEASING,
	) {
		for l.HoldNumber.Load() == RELEASING {
			runtime.Gosched()
		}

		return
	}

	slot := l.slot

	l.inside = ""
	l.holding = false
	l.slot = nil

	l.HoldNumber.Store(0)

	if slot != nil {
		<-slot
	}
}
