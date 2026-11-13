package scheduler

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// Within runs target on the caller's own goroutine and interpreter thread, as
// an evaluation of its own, and returns what it produced.
//
// An attempt of a repeat or a retry needs no goroutine of its own: its caller
// does nothing but wait for it. A goroutine, a thread, a context and its
// watch were most of what an attempt cost. So the evaluation borrows the
// caller's thread and carries its own locals there while it runs. The
// caller's locals go back when it ends, however it ends.
//
// It ends as an evaluation Fork starts does, through _Eval: with a
// failure Fail kept, a panic that ends the run, and a failure under a cancel
// reported as cancelled.
//
// Its context is the caller's. A cancel of the caller is then a cancel of
// this evaluation, and the caller's thread already watches for it. Nothing
// can cancel this evaluation alone: a cancel of an interpreter thread lasts,
// so it would stop the caller as well. An evaluation that its caller must be
// able to stop, as a timeout's must, runs Fork.
//
// Returns ErrRun when thread carries no run or target is nil. A nil option
// is skipped.
//
// Revisions:
//   - 2026-10-29 00:39: initial creation
//   - 2026-11-01 13:00: charges no budget, there being none
//   - 2026-11-02 00:48: takes the result in a _Result, with no Handle made to carry it
//   - 2026-11-04 23:50: a nil target is ErrNoCallee (C86 fix round 3)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:10: forwards to run.Within (C91w5)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun; uses _Eval, formerly _Evaluate (C121,
//     Q349, Q350)
//   - 2026-11-12 18:58: uses Fork and ForkOption, formerly Beside and BesideOption (C129, Q396)
func Within(
	thread *starlark.Thread,
	target starlark.Callable,
	opts ...common.ForkOption,
) (starlark.Value, error) {
	return run.Within(
		thread,
		target,
		opts...,
	)
}
