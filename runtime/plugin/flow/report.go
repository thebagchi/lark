package flow

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/scheduler"
)

// _Began tells whatever is watching that a wrapped function has begun another
// attempt.
//
// Unguarded here, and that is not an omission: these run inside a builtin,
// inside starlark.Call, so a panic is already caught by whatever guards that
// evaluation and ends the run. Guarding again would recover the same panic
// twice and say nothing more.
//
// The lane is the caller's, not a new one: Detach inherits a thread's number
// rather than taking one, so repeat(step, 3) is one entry on one lane advancing
// 1, 2, 3 rather than three lanes.
//
// Revisions:
//   - 2026-09-20 01:40: initial creation
func _Began(thread *starlark.Thread, name string, attempt int) {
	into := scheduler.Reporting(thread)
	if into == nil {
		return
	}

	into.Started(scheduler.Number(thread), name, int32(attempt))
}

// _Finished tells whatever is watching how a wrapped function ended.
//
// Called once per wrapper, not once per attempt, and that is the decision this
// file exists for. A caught failure is not the function failing - the whole
// point of retry is that attempt 2 failing is ordinary. Reporting every attempt
// would show a red function that is about to be fine to anyone polling in
// between, and would show a host two failures that never happened.
//
// scheduler.Catching already draws that line for assertions. This is the same
// line drawn for status.
//
// Revisions:
//   - 2026-09-20 01:40: initial creation
func _Finished(thread *starlark.Thread, name string, err error) {
	into := scheduler.Reporting(thread)
	if into == nil {
		return
	}

	into.Ended(scheduler.Number(thread), name, err)
}
