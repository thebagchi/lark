package core

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

var (
	// ERR_NOT_A_NAME is every refusal spawn makes of what it was handed: nothing,
	// something that is not a function, or a function taking an argument
	// spawn cannot supply.
	//
	// A lambda is not among them. A compiler emits one wherever a call passes
	// arguments - spawn(lambda: greet("alice")) - because spawn takes no
	// arguments to pass on. What a handle lost by accepting it is its name, and
	// what reports a run is the graph, which knows what runs on the lane a fork
	// named.
	ERR_NOT_A_NAME = errors.New("spawn wants a function")

	// ERR_CAPTURES is returned when something reaching a new thread captures a
	// local variable: a nested function spawned by name, a lambda made earlier
	// and stored, a closure handed over through a lambda.
	//
	// Such a function reads the variable when it runs rather than when it was
	// spawned, from a cell the thread that made it can go on writing - which
	// two goroutines did with nothing between them until 2026-09-29, when a
	// thread per item in a loop was found reading whichever item the loop had
	// reached. A lambda written inside spawn takes its variables at the spawn
	// instead, and is how a thread is handed what it needs.
	ERR_CAPTURES = errors.New("captures a local variable")
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
// Returns ERR_NOT_A_NAME when fn is missing, is not a function, or takes an
// argument spawn cannot supply, and ERR_CAPTURES naming the first captured
// variable found.
//
// Revisions:
//   - 2026-09-19 20:38: initial creation
//   - 2026-09-21 08:09: Beside with no options, which is a lane of its own
//   - 2026-09-21 09:46: moved here from the scheduler
//   - 2026-09-29 23:30: refuses anything reaching the thread that captures a
//     local variable, and freezes what the thread is handed
//   - 2026-10-02 00:48: takes the binding and the callee the dialect passes,
//     and labels the thread it starts with them
func _Spawn(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	binding, kwargs := scheduler.Hidden(kwargs, spelling.BINDING)
	callee, kwargs := scheduler.Hidden(kwargs, spelling.CALLEE)

	target, err := _Named(args, kwargs)
	if err != nil {
		return nil, err
	}

	err = _Walk(target, _Uncaptured)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SPAWN, err)
	}

	target.Freeze()

	return scheduler.Beside(thread, target, scheduler.Labelled(callee, binding))
}

// _Named returns the one function in args, or says why it is not one.
//
// Revisions:
//   - 2026-09-19 20:39: initial creation
//   - 2026-09-29 23:30: a parameter with a default is one spawn need not
//     supply, so a function whose parameters all have one is accepted
func _Named(args starlark.Tuple, kwargs []starlark.Tuple) (*starlark.Function, error) {
	if len(args) != 1 || len(kwargs) > 0 {
		return nil, fmt.Errorf("%s takes one function: %w", SPAWN, ERR_NOT_A_NAME)
	}

	target, ok := args[0].(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("%s got %s: %w", SPAWN, args[0].Type(), ERR_NOT_A_NAME)
	}

	if !_Defaulted(target) {
		return nil, fmt.Errorf("%s got %s, which takes arguments: %w",
			SPAWN, target.Name(), ERR_NOT_A_NAME)
	}

	return target, nil
}

// _Defaulted reports whether every parameter fn takes has a default.
//
// *args and **kwargs have none, so a function taking either is not.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Defaulted(fn *starlark.Function) bool {
	for idx := range fn.NumParams() {
		if fn.ParamDefault(idx) == nil {
			return false
		}
	}

	return true
}

// _Uncaptured refuses a function that captures a variable, unless it is one of
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
func _Uncaptured(fn *starlark.Function) error {
	if fn.NumFreeVars() == 0 || _Global(fn) {
		return nil
	}

	bind, _ := fn.FreeVar(0)

	return fmt.Errorf("%s captures %s, bound at %s: %w",
		fn.Name(), bind.Name, bind.Pos, ERR_CAPTURES)
}

// _Global reports whether fn is one of its module's globals: a top-level def,
// or a function a top-level name was bound to.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
func _Global(fn *starlark.Function) bool {
	for _, value := range fn.Globals() {
		if value == fn {
			return true
		}
	}

	return false
}
