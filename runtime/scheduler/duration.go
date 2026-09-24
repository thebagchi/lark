package scheduler

import (
	"errors"
	"fmt"
	"math"
	"time"

	"go.starlark.net/starlark"
)

// ErrDuration is returned for a duration that is not one, or that no timer can
// hold.
var ErrDuration = errors.New("not a duration")

const (
	// NANOS is how many of time.Duration's units make a second, so a script's
	// fractional seconds can become one.
	NANOS = float64(time.Second)

	// LIMIT is the first count of nanoseconds a timer cannot hold.
	//
	// Two to the sixty-third, written as that rather than as MaxInt64,
	// because a float64 cannot tell the two apart: converting MaxInt64 to a
	// float rounds it up to exactly this, so a guard reading "greater than
	// MaxInt64" lets this value through and time.Duration then wraps it to
	// the most negative duration there is. A sleep of that returns at once,
	// which is the opposite of what was asked for.
	LIMIT = 1 << 63
)

// Duration reads a script's number of seconds as a duration a timer can hold.
//
// Taken as a number rather than as a float, so sleep(1) works. One reader for
// sleep and timeout, refusing what time.Duration cannot represent: a float
// past the int64 range converts to a value the language leaves unspecified,
// and on amd64 it is a negative one, so sleep(1e300) returned at once.
//
// Returns ErrDuration for a value that is not a number, is negative, is not
// finite, or does not fit.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-09-24 16:08: refuses two to the sixty-third nanoseconds, which the
//     old guard compared against a float that had rounded up to meet it
func Duration(value starlark.Value) (time.Duration, error) {
	seconds, ok := starlark.AsFloat(value)
	if !ok {
		return 0, fmt.Errorf("got %s: %w", value.Type(), ErrDuration)
	}

	nanos := seconds * NANOS

	unfit := math.IsNaN(nanos) || math.IsInf(nanos, 0) || nanos < 0 || nanos >= LIMIT

	if unfit {
		return 0, fmt.Errorf("%g seconds: %w", seconds, ErrDuration)
	}

	return time.Duration(nanos), nil
}
