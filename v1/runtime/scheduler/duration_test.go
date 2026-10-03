package scheduler_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestDuration_ReadsWholeMilliseconds checks the durations a script may state:
// whole milliseconds, from none up to what the schema's int32 holds.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation, replacing the seconds this read before
func TestDuration_ReadsWholeMilliseconds(t *testing.T) {
	cases := map[string]struct {
		given starlark.Value
		want  time.Duration
	}{
		"nothing at all":      {starlark.MakeInt(0), 0},
		"a millisecond":       {starlark.MakeInt(1), time.Millisecond},
		"a second":            {starlark.MakeInt(1000), time.Second},
		"a second as a float": {starlark.Float(1000), time.Second},
		"the most an int32 is": {
			starlark.MakeInt(math.MaxInt32),
			math.MaxInt32 * time.Millisecond,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			held, err := scheduler.Duration(tc.given)
			if err != nil {
				t.Fatalf("%v gave %v", tc.given, err)
			}

			if held != tc.want {
				t.Fatalf("%v gave %v, want %v", tc.given, held, tc.want)
			}
		})
	}
}

// TestDuration_RefusesWhatTheSchemaCannotHold checks the durations a script may
// not state: a fraction of a millisecond, a negative one, more than an int32
// holds, and anything that is not a number.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation, replacing the seconds this read before
func TestDuration_RefusesWhatTheSchemaCannotHold(t *testing.T) {
	cases := map[string]starlark.Value{
		"half a millisecond":   starlark.Float(0.5),
		"a negative":           starlark.MakeInt(-1),
		"one past an int32":    starlark.MakeInt(math.MaxInt32 + 1),
		"far past everything":  starlark.Float(1e300),
		"not a number at all":  starlark.Float(math.NaN()),
		"a string of a number": starlark.String("1000"),
	}

	for name, given := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := scheduler.Duration(given)
			if !errors.Is(err, scheduler.ERR_DURATION) {
				t.Fatalf("%v gave %v, want ERR_DURATION", given, err)
			}
		})
	}
}
