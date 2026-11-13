package script

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// ARGS_LOCAL is what a run's arguments travel under on the thread a unit
// initialises on. A thread local is keyed by a string, so this is one. The
// thread holds it for as long as a module's top level runs, and Loads reads it
// as the mark of that time.
//
// Revisions:
//   - 2026-11-12 17:20: moved from artifact, which keeps the name (C128, B278)
const (
	ARGS_LOCAL = "artifact.args"
)

var (
	// ErrInit is returned when a builtin that starts script code on a thread of
	// its own - spawn and timeout - is called while a module's top level runs:
	// on the top level itself, in a function it calls, or in a module it loads.
	//
	// go.starlark.net shares a module between threads only after its top level
	// has run and its globals are frozen. A thread started earlier reads a
	// global the top level is writing, and a data race follows. The top level
	// has no thread to start anyway: main starts when it ends.
	//
	// It lives here because two sides refuse the same thing: the builtins, when
	// a script runs, and the flow check, when a flow holds the same statement.
	//
	// Revisions:
	//   - 2026-11-12 06:29: initial creation (C127, B274, Q383, Q388)
	ErrInit = errors.New("cannot start a thread while the module loads")
)

// Loading is the refusal of builtin name while a module's top level runs. The
// text names the builtin and ends with the hint of what to do. An empty name,
// for a caller that does not know the builtin, gives the text without it.
//
// Returns an error that matches ErrInit.
//
// Revisions:
//   - 2026-11-12 06:29: initial creation (C127, B274, Q389)
//   - 2026-11-12 17:20: an empty name gives a text without it (C128, B278)
func Loading(name string) error {
	if name == "" {
		return fmt.Errorf(
			"%w (call it from main or a function main calls)",
			ErrInit,
		)
	}

	return fmt.Errorf(
		"%s: %w (call %s from main or a function main calls)",
		name,
		ErrInit,
		name,
	)
}

// Starts reports whether the builtin name starts script code on a thread of its
// own, and so is refused while a module's top level runs (Q389). It is the one
// place the set is written: the flow check reads it, and a builtin that joins
// the set is added here.
//
// Revisions:
//   - 2026-11-12 06:36: initial creation (C127, B274, Q389)
func Starts(name string) bool {
	switch name {
	case common.SPAWN, common.TIMEOUT:
		return true
	}

	return false
}

// Loads reports whether thread runs the top level of a module: the thread a
// run binds its arguments to while its units initialise, before main runs. A
// thread nobody bound, or a nil thread, does not.
//
// It is the one mark of that time. A builtin that starts script code on a
// thread of its own reads it, and so does scheduler.Fork, which every such
// builtin calls (Q389).
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B278, Q383, Q389)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
//   - 2026-11-13 13:04: the arguments are workflow.Value (C132, Q437)
func Loads(thread *starlark.Thread) bool {
	if thread == nil {
		return false
	}

	_, ok := thread.Local(ARGS_LOCAL).(map[string]*workflowpb.Value)

	return ok
}
