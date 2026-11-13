// Package declare holds the arg builtin of the args plugin: the value a run
// supplied under a name, or the default a script stated. The plugin type, its
// registration and its names stay in package args.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package declare

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/plugin/args/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrRequired is returned for an argument a script declared with no
	// default and the run did not supply.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/args (C120b, Q315)
	//   - 2026-11-10 17:15: renamed from ErrNotSupplied (C121, Q349)
	ErrRequired = errors.New("argument not supplied and has no default")

	// ErrDeclare is returned for an arg() call somewhere a declaration
	// cannot be: inside a function body rather than at module level.
	//
	// Without it such a call is the worst of the outcomes available. It takes
	// its default in silence, because the thread running a body is not the
	// thread a run bound its arguments on, and deriving a graph does not carry
	// it either - so a script that looks like it reads an argument reads
	// nothing, twice over, invisibly.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/args (C120b, Q315)
	//   - 2026-11-10 17:15: renamed from ErrNotDeclaring (C121, Q349)
	ErrDeclare = errors.New("arg declares a module-level name")

	// ErrValue is returned for a supplied value this cannot hand a script,
	// which is a lark Value with no kind, or a null that holds something else.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/args (C120b, Q315)
	//   - 2026-11-10 17:15: renamed from ErrNotValue (C121, Q349)
	//   - 2026-11-13 12:50: a lark Value, in place of a google.protobuf.Value (C132, Q437)
	ErrValue = errors.New("argument is not a value")
)

// Declare is the builtin: the value the run supplied under name, or the
// default the script stated.
//
// Returns ErrRequired when neither exists, so a script declaring an
// argument it cannot do without fails at the line that declares it rather than
// somewhere later with None in its hands. Returns ErrDeclare when it is
// called anywhere but a module's top level.
//
// An attempt of a repeat or a retry is a body, though it runs on its caller's
// thread. At module level that thread is bound, so the attempt is refused by
// its count rather than by the thread.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-10-29 06:26: refuses a call inside an attempt
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-05 08:07: ErrNotSupplied ends with the -args example (C70)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/args and renamed _Declare to Declare, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrDeclare and ErrRequired, formerly ErrNotDeclaring and
//     ErrNotSupplied (C121, Q349)
func Declare(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name     string
		fallback starlark.Value
	)

	err := starlark.UnpackArgs(
		shared.Called(fn),
		args,
		kwargs,
		spec.SUPPLIED,
		&name,
		spec.DEFAULT+"?",
		&fallback,
	)
	if err != nil {
		return nil, err
	}

	var (
		supplied, bound = artifact.Supplied(thread)
		attempt         = scheduler.AttemptOf(thread)
		declaring       = bound && attempt == common.NO_ATTEMPT
	)
	if !declaring {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.ShortText(name),
			ErrDeclare,
		)
	}

	value, found := supplied[name]
	if found {
		return _Starlark(value)
	}

	if fallback == nil {
		return nil, fmt.Errorf(
			"%s: %w (supply it as -args '{%s: ...}', or give arg() a default)",
			unpack.ShortText(name),
			ErrRequired,
			unpack.QuotePrefix(name),
		)
	}

	return fallback, nil
}

// _Starlark is a supplied value as a script sees it.
//
// The conversion is unpack.FromValue: a lark Value of each kind is its own
// value, so an int stays an int and a float a float. A failure carries
// ErrValue, and the shared sentinel stays in the chain.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-11-02 11:24: converts with unpack.FromProto
//   - 2026-11-02 11:34: wraps a refusal with _Refused
//   - 2026-11-04 21:49: wraps the refusal itself, _Refused being gone (C59)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/args (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrValue, formerly ErrNotValue (C121, Q349)
//   - 2026-11-13 12:50: converts a lark Value with unpack.FromValue (C132, Q437)
func _Starlark(value *workflowpb.Value) (starlark.Value, error) {
	converted, err := unpack.FromValue(value)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %w",
			ErrValue,
			err,
		)
	}

	return converted, nil
}
