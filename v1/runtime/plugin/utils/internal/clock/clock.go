// Package clock holds the datetime builtin of the utils plugin.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package clock

import (
	"time"

	"go.starlark.net/starlark"

	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/utils/internal/spec"
)

// Datetime is the current local time as YYYY-MM-DD HH:MM:SS.uuuuuu.
//
// It returns the text rather than printing it, which is where this departs
// from the brief's print_datetime. A script that wants the line on its log
// writes print(utils.datetime()), and one that wants the same text in a state
// entry, a filename or a message can have it too - which a function that only
// printed could not give. One function, both uses, and the script says which.
//
// Local time, as the brief asks. That makes the text depend on the machine,
// which is why nothing in this repository compares it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as print_datetime
//   - 2026-09-21 15:25: returns the text rather than printing it, and is named
//     for what it gives back
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/utils and renamed _Datetime to Datetime,
//     for the tests in tests/ (C120b, Q315, Q314)
func Datetime(
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

	return starlark.String(time.Now().Format(spec.LAYOUT)), nil
}
