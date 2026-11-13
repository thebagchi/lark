package builtin

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// Recorded waits for every handle given and returns one record for each, in
// the order they were passed, whether the handle succeeded or not.
//
// join stops at the first failure and cancels the handles it has not reached.
// This does neither, and stops no handle: a script that wants to carry on
// after a thread it chose to ignore reads each record and decides. A record is
// a dict:
//
//	{"ok": bool, "value": v or None, "error": text or None, "kind": word or None}
//
// kind is "fail" for any thread error not listed next, "cancelled" for a
// handle that was cancelled, "cycle" for a value that held itself, and
// "nested" for a state operation inside an update. The error text is whole.
//
// A failure that ends the run does not become a record: an assert ends the run
// whoever waits.
//
// The rule for the caller's own cancellation: a record always comes from the
// handle, never from the caller's context. After the wait, a handle that is
// over gets its own result as its record, even when the caller was cancelled
// in the same moment (Wait chooses between the two at random, so its answer
// is not read). A handle that is not over, with the caller cancelled, raises
// scheduler.ErrCancelled, as join does, and no record is made up. The handles
// share the run's context, so the same cancel can end them: the caller then
// gets a "cancelled" record for each handle the cancel ended. Which of the
// two happens depends on timing, and neither is a false record.
//
// The arguments, the line the wait reports and the refusal inside an update
// are join's: see Lined.
//
// Returns ErrHandle when an argument is something else.
//
// Revisions:
//   - 2026-11-02 16:28: initial creation (C28)
//   - 2026-11-02 16:44: a finished handle is recorded from its own result,
//     not from what Wait answered (fix after the tester found false
//     "cancelled" records)
//   - 2026-11-02 16:59: the comment states both outcomes of a cancelled caller, with
//     no frequency word
//   - 2026-11-03 21:57: a nil handle is ErrNotAHandle (C52, B066)
//   - 2026-11-07 20:07: takes common.Handle in place of *scheduler.Handle (C91w5, Q127)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Recorded to Recorded,
//     for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrHandle, formerly ErrNotAHandle (C121, Q348)
func Recorded(
	thread *starlark.Thread,
	handles []common.Handle,
) (starlark.Value, error) {
	records := make(
		[]starlark.Value,
		0,
		len(handles),
	)

	for _, handle := range handles {
		if handle == nil {
			return nil, fmt.Errorf(
				"%w: nil handle",
				ErrHandle,
			)
		}

		var (
			_, waited = scheduler.Wait(
				thread,
				handle,
			)
			value, done, failure = handle.Result()
		)
		if !done {
			return nil, fmt.Errorf(
				"%s: %w",
				handle.Name(),
				waited,
			)
		}

		record, err := _Record(
			value,
			failure,
		)
		if err != nil {
			return nil, err
		}

		records = append(
			records,
			record,
		)
	}

	return starlark.NewList(records), nil
}

// _Record is the dict that stands for one finished handle.
//
// Revisions:
//   - 2026-11-02 16:28: initial creation (C28)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
func _Record(
	value starlark.Value,
	failure error,
) (starlark.Value, error) {
	var (
		message = starlark.Value(starlark.None)
		kind    = starlark.Value(starlark.None)
	)

	if value == nil || failure != nil {
		value = starlark.None
	}

	if failure != nil {
		message = starlark.String(failure.Error())
		kind = starlark.String(_Kind(failure))
	}

	record := starlark.NewDict(0)

	for _, field := range []struct {
		key   string
		value starlark.Value
	}{
		{
			"ok",
			starlark.Bool(failure == nil),
		},
		{
			"value",
			value,
		},
		{
			"error",
			message,
		},
		{
			"kind",
			kind,
		},
	} {
		err := record.SetKey(
			starlark.String(field.key),
			field.value,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"%s: %w",
				spec.OUTCOMES,
				err,
			)
		}
	}

	return record, nil
}

// _Kind is the kind word for an error a handle ended with. The order of the
// cases is part of the contract: the first that matches wins.
//
// Revisions:
//   - 2026-11-02 16:28: initial creation (C28)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrCyclic, formerly ErrSelfContaining (C121, Q349)
func _Kind(failure error) string {
	switch {
	case errors.Is(
		failure,
		scheduler.ErrCancelled,
	):
		return spec.KIND_CANCELLED

	case errors.Is(
		failure,
		scheduler.ErrNested,
	):
		return spec.KIND_NESTED

	case errors.Is(
		failure,
		trail.ErrCyclic,
	):
		return spec.KIND_CYCLE

	default:
		return spec.KIND_FAIL
	}
}
