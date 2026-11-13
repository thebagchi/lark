// Package attempt holds the attempt side of the flow plugin: the attempts of a
// repeat or a retry, their limits and their delays, and the repeat, retry and
// timeout builtins. The plugin type and its registration stay in package flow.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package attempt

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/flow/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrAttempt is returned when n() is called outside a wrapper.
	//
	// Revisions:
	//   - 2026-09-20 00:48: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow (C120b, Q315)
	ErrAttempt = errors.New("only meaningful inside repeat or retry")
)

// Attempt returns the 1-based attempt number of the repeat or retry this
// evaluation, or the evaluation that started it, is inside.
//
// Returns ErrAttempt outside one, rather than a number that would be a guess.
//
// Revisions:
//   - 2026-09-20 01:11: initial creation
//   - 2026-09-21 08:09: reads the attempt the scheduler carries, which a
//     spawned thread inherits
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _Attempt to Attempt, for
//     the tests in tests/ (C120b, Q315, Q314)
func Attempt(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		0,
	)
	if err != nil {
		return nil, err
	}

	attempt := scheduler.AttemptOf(thread)
	if attempt == common.NO_ATTEMPT {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			ErrAttempt,
		)
	}

	return starlark.MakeInt(int(attempt)), nil
}

// Repeat calls its target exactly count times, waiting the delay between
// calls when one is given.
//
// It stops at the first error and returns the last success. The same arguments
// go to every call: a repeat is for doing one thing several times, not for
// walking a list.
//
// Revisions:
//   - 2026-09-20 01:13: initial creation
//   - 2026-09-21 08:09: each attempt is Beside on the caller's lane
//   - 2026-10-02 01:01: takes an optional delay, and reports its line through
//     the scheduler with the count it makes
//   - 2026-10-29 00:23: names the call through _Written, built only on a
//     failure
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-08 00:27: names read from internal/spec and common (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/flow and renamed _Repeat to Repeat, for
//     the tests in tests/ (C120b, Q315, Q314)
func Repeat(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	attempts, err := _Counted(
		shared.Called(fn),
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	var last starlark.Value = starlark.None

	for attempt := int32(spec.ONCE); attempt <= attempts.count; attempt++ {
		last, err = attempts._Attempted(
			thread,
			attempt,
		)
		if err != nil {
			failed := fmt.Errorf(
				"%s attempt %d: %w",
				attempts._Written(),
				attempt,
				err,
			)

			scheduler.Close(
				thread,
				attempts.name,
				failed,
			)

			return nil, failed
		}
	}

	scheduler.Close(
		thread,
		attempts.name,
		nil,
	)

	return last, nil
}
