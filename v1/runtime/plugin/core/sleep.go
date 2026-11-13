package core

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Sleep pauses this evaluation for the given number of milliseconds.
//
// It waits on the evaluation's context as well as the clock, and that is the
// whole point of it. Cancelling a run cancels the interpreter thread, which
// takes effect between instructions - it does nothing at all to a Go call
// already blocked. A sleep written as a plain time.Sleep would therefore
// ignore a cancel, and a run would hang for the full duration rather than
// ending. Everything blocking in this runtime owes the same debt.
//
// The argument is milliseconds, the unit the schema stores. A sleep is a line
// of the thread that sleeps, reported around the wait.
//
// Returns scheduler.ErrCancelled when the evaluation ends first, so a script
// can tell a sleep that finished from one that was cut short.
//
// Revisions:
//   - 2026-09-20 01:02: initial creation
//   - 2026-09-21 08:09: reads its argument through Duration
//   - 2026-09-21 09:46: moved here from the scheduler
//   - 2026-10-02 00:48: reports itself as a line, and takes milliseconds
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 13:25: a cut sleep is scheduler.ErrCancelled, ErrInterrupted merged (C98, Q249)
func _Sleep(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	scheduler.Open(
		thread,
		&common.Line{
			Builtin: common.SLEEP,
		},
	)

	value, err := _Slept(
		thread,
		fn,
		args,
		kwargs,
	)

	scheduler.Close(
		thread,
		"",
		err,
	)

	return value, err
}

// _Slept reads the duration and waits it out, or until the evaluation ends.
//
// Revisions:
//   - 2026-10-02 00:48: initial creation, from _Sleep's body, so the line
//     _Sleep reports closes whichever way this ends
//   - 2026-11-10 11:32: uses builtin.Pause and builtin.Paused, which moved to internal/builtin
//     (C120b, Q315)
func _Slept(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	pause, err := builtin.Pause(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return builtin.Paused(
		thread,
		fn,
		pause,
	)
}
