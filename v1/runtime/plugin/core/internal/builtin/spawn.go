package builtin

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrClosure is returned when something reaching a new thread captures a
	// local variable: a nested function spawned by name, a lambda made earlier
	// and stored, a closure handed over through a lambda.
	//
	// Such a function reads the variable when it runs rather than when it was
	// spawned, from a cell the thread that made it can go on writing - which
	// two goroutines did with nothing between them until 2026-09-29, when a
	// thread per item in a loop was found reading whichever item the loop had
	// reached. A lambda written inside spawn takes its variables at the spawn
	// instead, and is how a thread is handed what it needs.
	//
	// Revisions:
	//   - 2026-09-30 20:58: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core (C120b, Q315)
	//   - 2026-11-10 17:15: renamed from ErrCaptures (C121, Q348)
	ErrClosure = errors.New("captures a local variable")
)

// Started freezes target and runs it beside the caller.
//
// Returns script.ErrInit, before anything starts, when thread is running the
// top level of a module: the run's arguments are bound on a thread for as long
// as that is so, and unbound before main runs.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 12:28: the walk takes the thread's context (C26)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Started to Started, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-11 22:50: refuses a spawn while the module's top level runs (C127, B274, Q383, Q389)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func Started(
	thread *starlark.Thread,
	target *starlark.Function,
	callee string,
	binding string,
) (starlark.Value, error) {
	_, loading := artifact.Supplied(thread)
	if loading {
		return nil, script.Loading(common.SPAWN)
	}

	err := _Walk(
		scheduler.Ambient(thread),
		target,
		Uncaptured,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			common.SPAWN,
			err,
		)
	}

	target.Freeze()

	return scheduler.Fork(
		thread,
		target,
		scheduler.Labelled(
			callee,
			binding,
		),
	)
}

// Defaulted reports whether every parameter fn takes has a default.
//
// *args and **kwargs have none, so a function taking either is not.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-04 00:30: a nil function takes no parameter, so every parameter has a default
//     (C54, B102)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Defaulted to Defaulted,
//     for the tests in tests/ (C120b, Q315, Q314)
func Defaulted(fn *starlark.Function) bool {
	if fn == nil {
		return true
	}

	for idx := range fn.NumParams() {
		if fn.ParamDefault(idx) == nil {
			return false
		}
	}

	return true
}

// Uncaptured refuses a function that captures a variable, unless it is one of
// its module's globals.
//
// A top-level function captures nothing but what its file loaded: a load binds
// a name local to the file, which a function reaches the way a closure reaches
// a local. A load is bound once, before anything runs, to a value its own
// module froze, so a global is exempt. Anything else that captures reads the
// variables of the function that made it, when it runs.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-04 00:30: a nil function is ErrNoReach (C54, B103)
//   - 2026-11-05 08:07: the text ends with CAPTURE_HINT (C70)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Uncaptured to
//     Uncaptured, for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrClosure, formerly ErrCaptures (C121, Q348)
func Uncaptured(fn *starlark.Function) error {
	if fn == nil {
		return _NoReach()
	}

	if fn.NumFreeVars() == 0 || Global(fn) {
		return nil
	}

	bind, _ := fn.FreeVar(0)

	return fmt.Errorf(
		"%s captures %s, bound at %s: %w (%s)",
		fn.Name(),
		bind.Name,
		bind.Pos,
		ErrClosure,
		spec.CAPTURE_HINT,
	)
}

// Global reports whether fn is one of its module's globals: a top-level def,
// or a function a top-level name was bound to.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-03 21:57: a nil function is the global of nothing (C52, B071)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/core and renamed _Global to Global, for
//     the tests in tests/ (C120b, Q315, Q314)
func Global(fn *starlark.Function) bool {
	if fn == nil {
		return false
	}

	for _, value := range fn.Globals() {
		if value == fn {
			return true
		}
	}

	return false
}
