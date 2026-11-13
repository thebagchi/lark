package core

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrFunction is every refusal spawn makes of what it was handed: nothing,
	// something that is not a function, or a function taking an argument
	// spawn cannot supply.
	//
	// A lambda is not among them. A compiler emits one wherever a call passes
	// arguments - spawn(lambda: greet("alice")) - because spawn takes no
	// arguments to pass on. What a handle lost by accepting it is its name, and
	// what reports a run is the graph, which knows what runs on the lane a fork
	// named.
	//
	// Revisions:
	//   - 2026-09-19 23:36: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 17:15: renamed from ErrNotAName (C121, Q348)
	ErrFunction = errors.New("wants a function")

	// ErrClosure is builtin.ErrClosure.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	//   - 2026-11-10 17:15: renamed from ErrCaptures (C121, Q348)
	ErrClosure = builtin.ErrClosure

	// ErrInit is script.ErrInit.
	//
	// Revisions:
	//   - 2026-11-11 22:50: initial creation (C127, B274, Q383, Q388)
	ErrInit = script.ErrInit
)

// _Spawn starts fn on a lane of its own and hands the script back a handle to
// it.
//
// A function callable with no arguments, because spawn passes none on: one
// taking no parameters, or one whose every parameter has a default. The second
// is what a lambda written inside spawn is once compiled - the dialect gives it
// a parameter per variable it reads, defaulting to that variable, so the lambda
// takes them at the spawn.
//
// Nothing reaching the new thread may capture a local variable, and all of it
// is frozen before the thread starts. A default holds the same object the
// variable did, so without the freeze the two threads would share a list with
// nothing between them; with it, a write from either fails loudly instead.
//
// The dialect passes two facts the source knows on the call itself, as keywords
// no script can write: the name the spawn was bound to, and the function a
// lambda calls. Both are taken out before the arguments are read, and handed on
// as the label the spawn is reported with.
//
// Returns ErrFunction when fn is missing, is not a function, or takes an
// argument spawn cannot supply, ErrClosure naming the first captured
// variable found, and ErrInit when a module's top level is running.
//
// Revisions:
//   - 2026-09-19 20:38: initial creation
//   - 2026-09-21 08:09: Beside with no options, which is a lane of its own
//   - 2026-09-21 09:46: moved here from the scheduler
//   - 2026-09-29 23:30: refuses anything reaching the thread that captures a
//     local variable, and freezes what the thread is handed
//   - 2026-10-02 00:48: takes the binding and the callee the dialect passes,
//     and labels the thread it starts with them
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 11:32: uses builtin.Started, which moved to internal/builtin (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrClosure and ErrFunction, formerly ErrCaptures and ErrNotAName
//     (C121, Q348)
//   - 2026-11-11 22:50: returns ErrInit while a module's top level runs (C127, B274, Q383)
func _Spawn(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		callee string
		target *starlark.Function
		err    error
	)

	binding, kwargs := scheduler.Hidden(
		kwargs,
		common.BINDING,
	)

	callee, kwargs = scheduler.Hidden(
		kwargs,
		common.CALLEE,
	)

	target, err = _Named(
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return builtin.Started(
		thread,
		target,
		callee,
		binding,
	)
}

// _Named returns the one function in args, or says why it is not one.
//
// Revisions:
//   - 2026-09-19 20:39: initial creation
//   - 2026-09-29 23:30: a parameter with a default is one spawn need not
//     supply, so a function whose parameters all have one is accepted
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 11:32: uses builtin.Defaulted, which moved to internal/builtin (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrFunction, formerly ErrNotAName (C121, Q348)
func _Named(
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*starlark.Function, error) {
	if len(args) != 1 || len(kwargs) > 0 {
		return nil, fmt.Errorf(
			"%s takes one function: %w",
			common.SPAWN,
			ErrFunction,
		)
	}

	target, ok := args[0].(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf(
			"%s got %s: %w",
			common.SPAWN,
			args[0].Type(),
			ErrFunction,
		)
	}

	if !builtin.Defaulted(target) {
		return nil, fmt.Errorf(
			"%s got %s, which takes arguments: %w",
			common.SPAWN,
			target.Name(),
			ErrFunction,
		)
	}

	return target, nil
}
