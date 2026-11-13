package core

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrHandle is builtin.ErrHandle.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	//   - 2026-11-10 17:15: renamed from ErrNotAHandle (C121, Q348)
	ErrHandle = builtin.ErrHandle

	// ErrSelf is builtin.ErrSelf.
	//
	// Revisions:
	//   - 2026-11-10 16:00: initial creation (C116, B237, Q327)
	//   - 2026-11-10 17:15: renamed from ErrSelfJoin (C121, Q348)
	ErrSelf = builtin.ErrSelf
)

// _Cancel stops every handle given, without waiting for any of them.
//
// Cancelling is not an outcome: a joined handle that was cancelled raises,
// because a caller asking for its result is asking for something that will
// never exist. A cancel is a line of the thread that wrote it.
//
// Revisions:
//   - 2026-09-19 20:44: initial creation
//   - 2026-09-21 09:46: moved here from the scheduler
//   - 2026-10-02 00:48: reports itself as a line
//   - 2026-11-02 00:29: closes its line once, as _Join does, with the error
//     _Stopped returns
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
func _Cancel(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	scheduler.Open(
		thread,
		&common.Line{
			Builtin: common.CANCEL,
		},
	)

	err := _Stopped(
		args,
		kwargs,
	)

	scheduler.Close(
		thread,
		"",
		err,
	)

	if err != nil {
		return nil, err
	}

	return starlark.None, nil
}

// _Stopped stops every handle given, once Handles has read them.
//
// Revisions:
//   - 2026-11-02 00:29: initial creation, from _Cancel's body, so the line
//     _Cancel reports closes in one place whichever way this ends
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 11:32: uses builtin.Handles, which moved to internal/builtin (C120b, Q315)
func _Stopped(
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	handles, err := builtin.Handles(
		common.CANCEL,
		args,
		kwargs,
	)
	if err != nil {
		return err
	}

	for _, handle := range handles {
		handle.Stop()
	}

	return nil
}
