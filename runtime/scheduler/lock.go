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
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-09-21 09:46: the name held is one string, since there is never a
//     second
func Lock(thread *starlark.Thread, name string) (func(), error) {
	locals, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	if locals.inside != "" {
		return nil, fmt.Errorf("already updating %q: %w", locals.inside, ErrNested)
	}

	slot := locals.run._Slot(name)

	select {
	case slot <- struct{}{}:
	case <-locals.ctx.Done():
		return nil, fmt.Errorf("waiting for %q: %w: %w", name, ErrCancelled, locals.ctx.Err())
	}

	locals.inside = name

	return func() {
		locals.inside = ""

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
