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

// NANOS is how many of time.Duration's units make a second, so a script's
// fractional seconds can become one.
const NANOS = float64(time.Second)

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
func Duration(value starlark.Value) (time.Duration, error) {
	seconds, ok := starlark.AsFloat(value)
	if !ok {
		return 0, fmt.Errorf("got %s: %w", value.Type(), ErrDuration)
	}

	nanos := seconds * NANOS

	unfit := math.IsNaN(nanos) || math.IsInf(nanos, 0) || nanos < 0 || nanos > math.MaxInt64

	if unfit {
		return 0, fmt.Errorf("%g seconds: %w", seconds, ErrDuration)
	}

	return time.Duration(nanos), nil
}
