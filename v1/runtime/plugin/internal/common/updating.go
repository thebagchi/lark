package common

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// NotUpdating refuses a wait while this evaluation holds a name.
//
// The thread that would post the event may need that name, and the wait
// lasts until the bound or until the run is cancelled.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _NotUpdating to
//     NotUpdating, for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-13 01:24: moved from v1/runtime/plugin/event/internal/latch to the shared
//     plugin/internal/common, so queue.pop calls it too (C131, Q417)
func NotUpdating(
	thread *starlark.Thread,
	fn *starlark.Builtin,
) error {
	updating, err := scheduler.Updating(thread)
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			Called(fn),
			err,
		)
	}

	if updating == "" {
		return nil
	}

	return fmt.Errorf(
		"%s inside an update of %s: %w",
		Called(fn),
		unpack.QuotePrefix(updating),
		scheduler.ErrNested,
	)
}
