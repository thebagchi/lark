package run

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Evaluation is the evaluation a builtin runs in, found on its thread once for
// the whole call.
//
// Every builtin here is handed only a thread, and finding the evaluation on it
// is a lookup in the thread's locals and an assertion. A builtin that needs
// the run's shared value and a lock paid that twice a call
// through Shared and Lock. In a profile of BenchmarkState_SetGet
// taken before this type, the lookup in the locals was about 5% of the time.
// A builtin that takes an Evaluation pays once.
//
// **A handle is valid only for the builtin call it was taken in.** Within
// puts an attempt's locals on the caller's thread while the attempt runs, and
// puts the caller's back when it ends. A handle taken inside an attempt is the
// attempt's, as a lookup at that moment would be. A handle kept past its call
// can name an evaluation the thread no longer runs, so a builtin takes a new
// one on each call and keeps none.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:00: now uses Locals, were _Locals (C120a, Q314)
type Evaluation struct {
	locals *Locals
}

// EvaluationOf returns the evaluation running on thread, for a builtin that
// needs more than one thing from it.
//
// Returns ErrRun when thread carries no run, as each function that takes a
// thread does.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-07 19:55: returns a common.Evaluation, nil when it errs (C91w5, Q119)
//   - 2026-11-10 01:00: now uses Of, were _Of (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func EvaluationOf(thread *starlark.Thread) (common.Evaluation, error) {
	locals, err := Of(thread)
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
// A key already held is read with no lock. The run holds its shared values in
// a map that is never changed after it is published; adding a key publishes a
// copy. Every call after the first for a key is a read, and sixteen threads
// reading the store of state once waited on the run's mutex (about half of
// the delay in BenchmarkState_GetManyThreads, measured before this change).
//
// build is called at most once per run per key, so two threads racing to be
// first still see one value. It runs with the run's mutex released, under one
// mutex for that key: build may call Shared for another key or start a thread,
// and only another caller of the same key waits for it. A build that asks for
// its own key, directly or through others, waits for itself.
//
// Returns ErrLocal when key already holds something of another type - which
// means two plugins chose one key.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation, from the body of Shared
//   - 2026-10-29 01:24: makes the run's shared map on its first value
//   - 2026-11-02 21:01: reads a held key with no lock (C39a)
//   - 2026-11-04 23:50: a nil or zero evaluation is ErrNoRun, and no build for a key not yet
//     held is ErrNoCallee (C86 fix round 3)
//   - 2026-11-07 19:55: takes a common.Evaluation; a nil one, or one this package did not make,
//     is ErrNoRun (C91w5, Q119)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Run, were run (C120a, Q314)
//   - 2026-11-10 11:52: build runs outside the run mutex, once for each key (C116, B238, Q296)
//   - 2026-11-10 17:15: uses ErrRun and ErrLocal, formerly ErrNoRun and ErrNotLocal (C121, Q349)
func SharedIn[T any](
	evaluation common.Evaluation,
	key string,
	build func() T,
) (T, error) {
	var empty T

	concrete, ok := evaluation.(*Evaluation)
	if !ok || concrete == nil || concrete.locals == nil {
		return empty, ErrRun
	}

	var (
		run         = concrete.locals.Run
		held, found = run._Held(key)
	)

	if !found && build == nil {
		return empty, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(key),
			ErrRun,
		)
	}

	if !found {
		held = run._Hold(
			key,
			func() any {
				return build()
			},
		)
	}

	value, ok := held.(T)
	if !ok {
		return empty, fmt.Errorf(
			"%s holds a %T: %w",
			key,
			held,
			ErrLocal,
		)
	}

	return value, nil
}

// Lock takes the run's lock for name on behalf of this evaluation, and returns
// the Hold that releases it. It is Lock for an evaluation already found,
// and holds its body: Lock tells why each refusal is what it is.
//
// Returns ErrNested when this evaluation took a name, or was started inside an
// update, and ErrCancelled wrapping the context's error when the wait is
// cancelled.
//
// The Hold returned frees this hold only. Released again, or after a later
// Lock, it does nothing.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation, from the body of Lock
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-04 00:30: an evaluation with no locals, or none at all, is ErrNoRun (C54, B089)
//   - 2026-11-04 23:50: a nil or zero evaluation is ErrNoRun (C86 fix round 3)
//   - 2026-11-05 22:05: the release is made for each hold, and frees that hold (C58 fix 1, Q85)
//   - 2026-11-05 22:40: returns a Hold, a value with no allocation (C58 fix 2, Q201)
//   - 2026-11-07 19:55: returns a common.Hold, since Hold moved to common as data (C91w5, Q229)
//   - 2026-11-10 01:00: now uses Run, Take, Ctx, HoldNumber, were run, _Take, ctx, hold
//     (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func (e *Evaluation) Lock(name string) (common.Hold, error) {
	if e == nil || e.locals == nil {
		return common.Hold{}, ErrRun
	}

	locals := e.locals

	if locals.holding {
		return common.Hold{}, fmt.Errorf(
			"already updating %s: %w",
			unpack.QuotePrefix(locals.inside),
			ErrNested,
		)
	}

	if locals.inside != "" {
		return common.Hold{}, fmt.Errorf(
			"started inside an update of %s: %w",
			unpack.QuotePrefix(locals.inside),
			ErrNested,
		)
	}

	var (
		slot = locals.Run._Slot(name)
		err  = Take(
			locals.Ctx,
			slot,
			name,
		)
	)
	if err != nil {
		return common.Hold{}, err
	}

	locals.inside = name
	locals.holding = true
	locals.slot = slot

	locals.issued++

	token := locals.issued

	locals.HoldNumber.Store(token)

	return common.Hold{
		Owner: locals,
		Token: token,
	}, nil
}
