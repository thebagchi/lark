package scheduler_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestDuration_RefusesWhatATimerCannotHold is the boundary a float cannot see.
//
// A float64 has no value between two to the sixty-third and the largest int64,
// so converting MaxInt64 to a float rounds it up to exactly two to the
// sixty-third - and a guard written as "greater than MaxInt64" therefore let
// that value through, where time.Duration wrapped it to the most negative
// duration there is. A sleep of that returns at once, which is the opposite of
// what the script asked for.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
func TestDuration_RefusesWhatATimerCannotHold(t *testing.T) {
	// The exact value the guard used to admit: two to the sixty-third
	// nanoseconds, as a count of seconds.
	overflow := starlark.Float(math.Ldexp(1, 63) / float64(time.Second))

	_, err := scheduler.Duration(overflow)
	if !errors.Is(err, scheduler.ErrDuration) {
		t.Fatalf("%v seconds gave %v, want ErrDuration", overflow, err)
	}

	// And the rounding is real: this is the value that used to pass the
	// comparison it should have failed.
	if !(float64(math.MaxInt64) == math.Ldexp(1, 63)) {
		t.Fatal("MaxInt64 no longer rounds to 2^63 as a float, so this guard needs rereading")
	}
}

// TestDuration_KeepsWhatATimerCanHold checks the fix did not close the door on
// ordinary durations, including the largest one that fits.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
func TestDuration_KeepsWhatATimerCanHold(t *testing.T) {
	cases := map[string]starlark.Value{
		"a second":        starlark.MakeInt(1),
		"a millisecond":   starlark.Float(0.001),
		"nothing at all":  starlark.MakeInt(0),
		"the largest fit": starlark.Float(math.Nextafter(math.Ldexp(1, 63), 0) / float64(time.Second)),
	}

	for name, given := range cases {
		t.Run(name, func(t *testing.T) {
			held, err := scheduler.Duration(given)
			if err != nil {
				t.Fatalf("%v gave %v", given, err)
			}

			if held < 0 {
				t.Fatalf("%v gave a negative duration, %v", given, held)
			}
		})
	}
}
