package builtin

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"

	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// Paused sleeps for pause, and stops when the caller's run is cancelled.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-06 06:45: names a nil builtin through the shared Called (C101, B182)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Paused to Paused, for
//     the tests in tests/ (C120b, Q315, Q314)
func Paused(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	pause time.Duration,
) (starlark.Value, error) {
	ctx, err := scheduler.Context(thread)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	timer := time.NewTimer(pause)
	defer timer.Stop()

	select {
	case <-timer.C:
		return starlark.None, nil

	case <-ctx.Done():
		return nil, fmt.Errorf(
			"%s: sleep was interrupted: %w: %w",
			shared.Called(fn),
			scheduler.ErrCancelled,
			ctx.Err(),
		)
	}
}

// Pause reads the milliseconds a sleep was given.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-03 21:57: names a nil builtin ANONYMOUS (C52, B070)
//   - 2026-11-06 00:20: names a nil builtin through the shared Called (C100b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Pause to Pause, for the
//     tests in tests/ (C120b, Q315, Q314)
func Pause(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (time.Duration, error) {
	var given starlark.Value

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&given,
	)
	if err != nil {
		return 0, err
	}

	pause, err := scheduler.Duration(given)
	if err != nil {
		return 0, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return pause, nil
}
