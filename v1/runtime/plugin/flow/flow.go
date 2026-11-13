// Package flow gives a script the wrappers that repeat, retry and bound a call.
// Importing it is what enables it.
//
// Each wrapper takes a count or a budget first and the function second, and
// calls it straight away: what it gives back is what the wrapped call
// produced. A repeat or a retry may take a delay third, the milliseconds it
// waits between calls, and a timeout's budget is milliseconds too. Every
// attempt runs beside the caller on a thread of its own, on the caller's own
// lane, through the scheduler's one primitive for that.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package flow

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/flow/internal/attempt"
	"github.com/thebagchi/lark/v1/runtime/plugin/flow/internal/spec"
)

var (
	// ErrAttempt is attempt.ErrAttempt.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrAttempt = attempt.ErrAttempt
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation
func init() {
	plugin.Register(&_Flow{})
}

// _Flow is the plugin. Empty: a wrapper holds what it needs, so there is
// nothing here.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Flow struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation
//   - 2026-11-08 00:27: names read from internal/spec and common (C91w12a)
func (f *_Flow) Name() string {
	return spec.NAME
}

// Values returns the four names this plugin supplies.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation
//   - 2026-11-08 00:27: names read from internal/spec and common (C91w12a)
//   - 2026-11-10 11:32: uses attempt.Repeat, attempt.Retry, attempt.Timeout and
//     attempt.Attempt, which moved to internal/attempt (C120b, Q315)
func (f *_Flow) Values() starlark.StringDict {
	return starlark.StringDict{
		common.REPEAT: starlark.NewBuiltin(
			common.REPEAT,
			attempt.Repeat,
		),
		common.RETRY: starlark.NewBuiltin(
			common.RETRY,
			attempt.Retry,
		),
		common.TIMEOUT: starlark.NewBuiltin(
			common.TIMEOUT,
			attempt.Timeout,
		),
		spec.ATTEMPT: starlark.NewBuiltin(
			spec.ATTEMPT,
			attempt.Attempt,
		),
	}
}
