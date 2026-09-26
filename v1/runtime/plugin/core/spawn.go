package core

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ErrNotAName is every refusal spawn can make: the argument is missing, is
// not a function, or takes parameters nothing can supply.
//
// A lambda is not among them. A compiler emits one wherever a call passes
// arguments - spawn(lambda: greet("alice")) - because spawn takes no arguments
// to pass on. What a handle lost by accepting it is its name, and what reports
// a run is the graph, which knows what runs on the lane a fork named.
var ErrNotAName = errors.New("spawn wants a function")

// _Spawn starts fn on a lane of its own and hands the script back a handle to
// it.
//
// A function taking no parameters, because spawn passes none on.
//
// Returns ErrNotAName when fn is missing, is not callable, or takes arguments.
//
// Revisions:
//   - 2026-09-19 20:38: initial creation
//   - 2026-09-21 08:09: Beside with no options, which is a lane of its own
//   - 2026-09-21 09:46: moved here from the scheduler
func _Spawn(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	target, err := _Named(args, kwargs)
	if err != nil {
		return nil, err
	}

	return scheduler.Beside(thread, target)
}

// _Named returns the one function in args, or says why it is not one.
//
// Revisions:
//   - 2026-09-19 20:39: initial creation
func _Named(args starlark.Tuple, kwargs []starlark.Tuple) (*starlark.Function, error) {
	if len(args) != 1 || len(kwargs) > 0 {
		return nil, fmt.Errorf("%s takes one function: %w", SPAWN, ErrNotAName)
	}

	target, ok := args[0].(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("%s got %s: %w", SPAWN, args[0].Type(), ErrNotAName)
	}

	if target.NumParams() > 0 {
		return nil, fmt.Errorf("%s got %s, which takes arguments: %w", SPAWN, target.Name(), ErrNotAName)
	}

	return target, nil
}
