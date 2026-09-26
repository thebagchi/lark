package scheduler

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// ErrNested is returned when an evaluation inside an update, or anything that
// evaluation started, begins another.
var ErrNested = errors.New("an update cannot start another")

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
func Lock(thread *starlark.Thread, name string) (func(), error) {
	locals, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	if locals.holding {
		return nil, fmt.Errorf("already updating %q: %w", locals.inside, ErrNested)
	}

	if locals.inside != "" {
		return nil, fmt.Errorf("started inside an update of %q: %w", locals.inside, ErrNested)
	}

	slot := locals.run._Slot(name)

	select {
	case slot <- struct{}{}:
	case <-locals.ctx.Done():
		return nil, fmt.Errorf("waiting for %q: %w: %w", name, ErrCancelled, locals.ctx.Err())
	}

	locals.inside = name
	locals.holding = true

	return func() {
		locals.inside = ""
		locals.holding = false

		<-slot
	}, nil
}

// _Slot returns the one-place channel that is the lock for name, making it the
// first time it is asked for.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func (r *_Run) _Slot(name string) chan struct{} {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	slot, found := r.locks[name]
	if !found {
		slot = make(chan struct{}, 1)
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
