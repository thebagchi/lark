package scheduler

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// EvaluationOf returns the evaluation running on thread, for a builtin that
// needs more than one thing from it.
//
// Returns ErrRun when thread carries no run, as each function that takes a
// thread does.
//
// Revisions:
//   - 2026-10-29 01:05: initial creation
//   - 2026-11-07 20:10: forwards to run.EvaluationOf (C91w5)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func EvaluationOf(thread *starlark.Thread) (common.Evaluation, error) {
	return run.EvaluationOf(thread)
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
//   - 2026-11-07 20:10: forwards to run.SharedIn (C91w5)
//   - 2026-11-10 11:52: build runs outside the run mutex, once for each key (C116, B238, Q296)
//   - 2026-11-10 17:15: uses ErrLocal, formerly ErrNotLocal (C121, Q349)
func SharedIn[T any](
	evaluation common.Evaluation,
	key string,
	build func() T,
) (T, error) {
	return run.SharedIn(
		evaluation,
		key,
		build,
	)
}
