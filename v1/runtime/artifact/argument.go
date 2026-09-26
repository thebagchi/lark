package artifact

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"
)

// ErrNotObject is returned when what a run was given is not a JSON object.
var ErrNotObject = errors.New("arguments are a JSON object")

// ARGS_LOCAL is what a run's arguments travel under on the thread a unit
// initialises on. A thread local is keyed by a string, so this is one.
const ARGS_LOCAL = "artifact.args"

// _Supply is what a run's arguments travel under on a context. A type rather
// than a string, so nothing else can write the key by accident.
type _Supply struct{}

// WithArgs is ctx carrying what a run supplies its script's arguments with.
//
// On the context rather than a parameter of Run, because everything else a run
// is told travels this way - the reporter, the printer - and the host handing a
// run its arguments is the host handing it those.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func WithArgs(ctx context.Context, supplied map[string]*structpb.Value) context.Context {
	return context.WithValue(ctx, _Supply{}, supplied)
}

// Supplied is what the run this thread initialises for supplied, and whether
// this thread is one a run bound at all.
//
// The second answer is what separates a run that supplied nothing, where every
// declaration takes its default, from a thread running a function body, where
// no value could ever arrive. Every unit a run initialises is bound, with or
// without values; a thread nobody bound is running a body.
//
// Exported for the plugin that reads it, the way the scheduler exports what a
// store belongs to. A plugin holding per-run state reaches whoever owns the
// thread, and the thread a module initialises on is this package's.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func Supplied(thread *starlark.Thread) (map[string]*structpb.Value, bool) {
	held, ok := thread.Local(ARGS_LOCAL).(map[string]*structpb.Value)

	return held, ok
}

// _Bind puts a run's arguments on the thread a unit initialises on.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func _Bind(thread *starlark.Thread, ctx context.Context) {
	supplied, _ := ctx.Value(_Supply{}).(map[string]*structpb.Value)

	if supplied == nil {
		supplied = map[string]*structpb.Value{}
	}

	thread.SetLocal(ARGS_LOCAL, supplied)
}

// Parsed is the arguments a JSON object states.
//
// A JSON object because that is what a caller writes on a command line and
// what a user interface already holds, and because its members are values,
// which is the limit a graph carries.
//
// Returns ErrNotObject for anything else, and a wrapped error for JSON that
// does not parse.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func Parsed(src []byte) (map[string]*structpb.Value, error) {
	held := new(structpb.Value)

	err := held.UnmarshalJSON(src)
	if err != nil {
		return nil, fmt.Errorf("arguments: %w", err)
	}

	object := held.GetStructValue()
	if object == nil {
		return nil, ErrNotObject
	}

	return object.GetFields(), nil
}
