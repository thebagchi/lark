package scheduler

import (
	"fmt"

	"go.starlark.net/starlark"
)

// Evaluation is the evaluation a builtin runs in, found on its thread once for
// the whole call.
//
// Every builtin here is handed only a thread, and finding the evaluation on it
// is a lookup in the thread's locals and an assertion. A builtin that needs
// the run's shared value, its budget and a lock paid that three times a call
// through Shared, Allowance and Lock. In a profile of BenchmarkState_SetGet
// taken before this type, the lookup in the locals was about 5% of the time.
// A builtin that takes an Evaluation pays once.
//
// **A handle is valid only for the builtin call it was taken in.** Within
// puts an attempt's locals on the caller's thread while the attempt runs, and
// puts the caller's back when it ends. A handle taken inside an attempt is the
// attempt's, as a lookup at that moment would be. A handle kept past its call
// can name an evaluation the thread no longer runs, so a builtin takes a new
// one on each call and keeps none.
type Evaluation struct {
	locals *_Locals
}

// EvaluationOf returns the evaluation running on thread, for a builtin that
// needs more than one thing from it.
//
// Returns ErrNoRun when thread carries no run, as each function that takes a
// thread does.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
func EvaluationOf(thread *starlark.Thread) (*Evaluation, error) {
	locals, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	return &Evaluation{
		locals: locals,
	}, nil
}

// SharedIn returns the run's value for key, building it the first time it is
// asked for. It is Shared for an evaluation already found, and holds its body.
//
// A function and not a method, because a method of Go cannot have a type
// parameter of its own, and the type parameter is what keeps the assertion
// out of every caller.
//
// build is called at most once per run per key, under the lock, so two threads
// racing to be first still see one value.
//
// Returns ErrNotLocal when key already holds something of another type - which
// means two plugins chose one key.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation, from the body of Shared
//   - 2026-10-29 01:24: makes the run's shared map on its first value
func SharedIn[T any](
	evaluation *Evaluation,
	key string,
	build func() T,
) (T, error) {
	var empty T

	run := evaluation.locals.run

	run.mutex.Lock()
	defer run.mutex.Unlock()

	held, found := run.shared[key]
	if !found {
		if run.shared == nil {
			run.shared = make(map[string]any)
		}

		made := build()
		run.shared[key] = made

		return made, nil
	}

	value, ok := held.(T)
	if !ok {
		return empty, fmt.Errorf(
			"%s holds a %T: %w",
			key,
			held,
			ErrNotLocal,
		)
	}

	return value, nil
}

// Allowance is the budget the run of this evaluation works to. Every
// evaluation of one run gets the same one, because they share the memory of
// one process.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
func (e *Evaluation) Allowance() *Budget {
	return e.locals.run.budget
}

// Lock takes the run's lock for name on behalf of this evaluation, and returns
// the function that releases it. It is Lock for an evaluation already found,
// and holds its body: Lock tells why each refusal is what it is.
//
// Returns ErrNested when this evaluation took a name, or was started inside an
// update, and ErrCancelled wrapping the context's error when the wait is
// cancelled.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation, from the body of Lock
func (e *Evaluation) Lock(name string) (func(), error) {
	locals := e.locals

	if locals.holding {
		return nil, fmt.Errorf(
			"already updating %q: %w",
			locals.inside,
			ErrNested,
		)
	}

	if locals.inside != "" {
		return nil, fmt.Errorf(
			"started inside an update of %q: %w",
			locals.inside,
			ErrNested,
		)
	}

	var (
		slot = locals.run._Slot(name)
		err  = _Take(
			locals.ctx,
			slot,
			name,
		)
	)
	if err != nil {
		return nil, err
	}

	locals.inside = name
	locals.holding = true
	locals.slot = slot

	if locals.release == nil {
		locals.release = locals._Release
	}

	return locals.release, nil
}
