package run

import (
	"errors"
	"fmt"
	"math"
	"time"

	"go.starlark.net/starlark"
)

// ErrDuration is returned for a duration that is not one: not a number,
// negative, not a whole number of milliseconds, or more than the schema's
// int32 can hold.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
var (
	ErrDuration = errors.New("not a duration")
)

// Duration reads a script's number of milliseconds as a duration a timer can
// hold.
//
// Milliseconds because that is what the schema stores, so a script and its
// flow say the same number: sleep(1000) is a second, and durationMs is 1000.
// One reader for sleep, timeout and the repeat and retry delay. A whole number
// that fits an int32, so a flow can carry every duration a script states, and
// a timer never sees a value time.Duration cannot represent.
//
// Taken as a number rather than as an int, so sleep(1000.0) is a second too.
//
// Returns ErrDuration for a value that is not a number, is negative, is not a
// whole number of milliseconds, or is past MAX_MILLIS.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-09-24 16:08: refuses two to the sixty-third nanoseconds, which the
//     old guard compared against a float that had rounded up to meet it
//   - 2026-10-02 00:38: reads milliseconds rather than seconds, and refuses a
//     fraction of one or more than an int32 holds
//   - 2026-11-04 00:30: a nil value is "got nil", not a panic (C54, B087)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
func Duration(value starlark.Value) (time.Duration, error) {
	if value == nil {
		return 0, fmt.Errorf(
			"got nil: %w",
			ErrDuration,
		)
	}

	millis, ok := starlark.AsFloat(value)
	if !ok {
		return 0, fmt.Errorf(
			"got %s: %w",
			value.Type(),
			ErrDuration,
		)
	}

	whole := millis >= 0 && millis <= MAX_MILLIS && millis == math.Trunc(millis)
	if !whole {
		return 0, fmt.Errorf(
			"%g milliseconds: %w",
			millis,
			ErrDuration,
		)
	}

	return time.Duration(millis) * time.Millisecond, nil
}
