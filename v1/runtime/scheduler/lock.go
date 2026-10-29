package scheduler

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// ErrNested is returned when an evaluation inside an update, or anything that
// evaluation started, begins another.
var (
	ErrNested = errors.New("an update cannot start another")
)

// Lock takes the run's lock for name on behalf of the evaluation on thread, and
// returns the function that releases it.
//
// It waits on the evaluation's context as well as the lock, which a
// sync.Mutex cannot: a run cancelled while a thread waits for a name another
// thread holds must end, not hang.
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
func Lock(
	thread *starlark.Thread,
	name string,
) (func(), error) {
	evaluation, err := EvaluationOf(thread)
	if err != nil {
		return nil, err
	}

	return evaluation.Lock(name)
}

// _Slot returns the one-place channel that is the lock for name, making it the
// first time it is asked for.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-10-29 01:24: makes the lock map on the run's first lock
func (r *_Run) _Slot(name string) chan struct{} {
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
func Updating(thread *starlark.Thread) (string, error) {
	locals, err := _Of(thread)
	if err != nil {
		return "", err
	}

	return locals.inside, nil
}

// _Take puts the one token in slot, and waits for room only when the slot is
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
func _Take(
	ctx context.Context,
	slot chan struct{},
	name string,
) error {
	select {
	case slot <- struct{}{}:
		return nil
	default:
	}

	select {
	case slot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf(
			"waiting for %q: %w: %w",
			name,
			ErrCancelled,
			ctx.Err(),
		)
	}
}

// _Release gives back the slot the evaluation holds, and clears the mark.
//
// It reads the slot from the evaluation rather than from a closure, so one
// function value serves every lock the evaluation takes. That is safe because
// an evaluation holds at most one name: a second lock is refused while it
// holds one.
//
// Revisions:
//   - 2026-10-28 22:06: initial creation
func (l *_Locals) _Release() {
	slot := l.slot

	l.inside = ""
	l.holding = false
	l.slot = nil

	<-slot
}
