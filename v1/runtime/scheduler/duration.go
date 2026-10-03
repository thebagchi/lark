package scheduler

import (
	"errors"
	"fmt"
	"math"
	"time"

	"go.starlark.net/starlark"
)

// ERR_DURATION is returned for a duration that is not one: not a number,
// negative, not a whole number of milliseconds, or more than the schema's
// int32 can hold.
var ERR_DURATION = errors.New("not a duration")

// MAX_MILLIS is the longest duration a script may ask for, in milliseconds:
// what the schema's int32 can hold, about 24.8 days.
const MAX_MILLIS = math.MaxInt32

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
// Returns ERR_DURATION for a value that is not a number, is negative, is not a
// whole number of milliseconds, or is past MAX_MILLIS.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-09-24 16:08: refuses two to the sixty-third nanoseconds, which the
//     old guard compared against a float that had rounded up to meet it
//   - 2026-10-02 00:38: reads milliseconds rather than seconds, and refuses a
//     fraction of one or more than an int32 holds
func Duration(value starlark.Value) (time.Duration, error) {
	millis, ok := starlark.AsFloat(value)
	if !ok {
		return 0, fmt.Errorf("got %s: %w", value.Type(), ERR_DURATION)
	}

	whole := millis >= 0 && millis <= MAX_MILLIS && millis == math.Trunc(millis)
	if !whole {
		return 0, fmt.Errorf("%g milliseconds: %w", millis, ERR_DURATION)
	}

	return time.Duration(millis) * time.Millisecond, nil
}
