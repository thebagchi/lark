package artifact

import (
	"errors"
	"fmt"
	"io"
	"log/slog"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ARGS_LOCAL is what a run's arguments travel under on the thread a unit
// initialises on. A thread local is keyed by a string, so this is one.
const (
	ARGS_LOCAL = "artifact.args"
)

// ErrNotJSON is returned for a run handed arguments holding a value JSON
// cannot: a function, a channel, a struct. Arguments are values, which is the
// limit a flow carries.
var (
	ErrNotJSON = errors.New("an argument holds a value JSON cannot")
)

// RunOption sets one thing on a run of an artifact: the arguments its script
// is handed, the logger what it prints goes to, the memory it may hold, or
// where its changes go.
//
// An option rather than a parameter of Run, so a run can be told more without
// every caller changing. What travels on the context is only when to stop.
type RunOption func(given *Given)

// Given is what a run's options told it: the arguments its script is supplied
// with, the logger what it prints goes to, the memory it may hold, the
// function its graph's changes go to, and what watches it, each zero when
// nothing was. refused is why the arguments could not be supplied, which an
// option cannot return and the run does, before anything runs.
//
// Exported for observe, which reads the logger and the watcher before a run
// starts and adds what watches it: it applies the options once, with Apply,
// and hands the result to Perform, where each of those steps applied them all
// again into a new one. Its fields stay this package's, so the one way to set
// one is an option.
type Given struct {
	args    *structpb.Struct
	refused error
	logger  *slog.Logger
	ceiling int64
	watch   func(change *workflowpb.Change)
	into    scheduler.Reporter
}

// WithArgs supplies a run's script with its arguments: each entry of args is
// the value of the argument the script declares under that name.
//
// The script's, not the runtime's, so a run is handed them rather than finding
// them on its context. Each run of one artifact may be handed different ones,
// because each run initialises the script again. A map of Go values, which is
// what a host holds - JSON it has read with encoding/json included - and each
// must be one JSON can hold, which is the limit a flow carries. One that is not
// fails the run when it starts, with ErrNotJSON, since an option has nowhere
// to say so.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-10-02 13:12: reads the JSON object itself, so a host supplies
//     arguments in one call
//   - 2026-10-02 16:08: an option of a run, rather than a context carrying the
//     arguments
//   - 2026-10-02 16:18: takes a Struct, a JSON object by its type, rather than
//     text to parse
//   - 2026-10-03 16:30: takes a map of Go values, building the Struct itself, so a
//     host imports nothing to supply them
func WithArgs(args map[string]any) RunOption {
	held, err := structpb.NewStruct(args)
	if err != nil {
		err = fmt.Errorf(
			"%w: %w",
			ErrNotJSON,
			err,
		)
	}

	return func(given *Given) {
		given.args = held
		given.refused = err
	}
}

// WithLogger sends what a run's script prints to logger: one record a line,
// the line as the message, with the thread and the function that printed it as
// attributes. Without it, the lines go to slog's default logger.
//
// A logger, so the handler a host chose writes the time and the layout - text
// or JSON, to a terminal, a file or something that streams - rather than a
// format of this runtime's. Each run of one artifact may log somewhere else.
//
// Revisions:
//   - 2026-10-02 16:21: initial creation
func WithLogger(logger *slog.Logger) RunOption {
	return func(given *Given) {
		given.logger = logger
	}
}

// WithLog writes what a run's script prints to w, one line a print: the time,
// the level, the thread and the function that printed it, and then the line,
// as the lark command prints them.
//
// The plain way to keep a run's output, for a host with nothing but a writer;
// a host with a slog handler of its own passes it to WithLogger instead. The
// message is last, so a line says where it came from before what was said,
// which slog's own handlers do the other way round.
//
// Revisions:
//   - 2026-10-03 16:31: initial creation
func WithLog(w io.Writer) RunOption {
	return WithLogger(slog.New(_NewTrailing(w)))
}

// WithMemory holds a run to bytes of what this runtime allocates on its
// script's behalf, rather than the default ceiling, scheduler.CEILING.
//
// It bounds what the library allocates for the run - what it reads, stores,
// copies and spawns - and not the interpreter's own allocations. Chosen before
// the run starts and never after, because a ceiling a script could raise
// partway through is not a ceiling. Zero or less is the default.
//
// Revisions:
//   - 2026-10-03 08:31: initial creation, replacing the ceiling a context carried
func WithMemory(bytes int64) RunOption {
	return func(given *Given) {
		given.ceiling = bytes
	}
}

// Reporting has a run tell into what it does: each line that starts and ends.
//
// For observe, which keeps a started run's graph, and for a test watching a
// run's lines; a host watches through WithChanges instead.
//
// Revisions:
//   - 2026-10-03 08:31: initial creation, replacing the reporter a context carried
func Reporting(into scheduler.Reporter) RunOption {
	return func(given *Given) {
		given.into = into
	}
}

// WithChanges sends a started run's graph, as it changes, to watch: one Change
// a step, the JSON Patch that takes a copy of the graph from before the step to
// after it, from the first change, which makes an empty graph running.
//
// watch is called on the goroutine of the thread whose step it was, one change
// at a time and in the order they were made, and no other step is folded while
// it runs, so a slow one holds the run up. Asked from inside, the run's whole
// graph is exactly the graph after the change being told.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func WithChanges(watch func(change *workflowpb.Change)) RunOption {
	return func(given *Given) {
		given.watch = watch
	}
}

// Changes is the function opts send a started run's changes to, or nil.
//
// Exported for observe, which keeps a started run's graph and is what sends
// them; a run made here keeps none. A caller that also runs with opts applies
// them once, with Apply, and asks the Given instead.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func Changes(opts []RunOption) func(change *workflowpb.Change) {
	return Apply(opts).Changes()
}

// Logger is the logger opts send a run's prints to, or nil when they name
// none and the prints go to slog's default.
//
// Exported for observe, which keeps each line a run prints as well as logging
// it, and so must log it where the run would have. A caller that also runs
// with opts applies them once, with Apply, and asks the Given instead.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func Logger(opts []RunOption) *slog.Logger {
	return Apply(opts).Logger()
}

// Apply is what opts tell a run, each applied in order, so of two that set one
// thing the later wins.
//
// A new Given each call, which belongs to the one run it is handed to: an
// option a caller applies to it afterwards changes that run alone.
//
// Revisions:
//   - 2026-10-02 16:08: initial creation, as _Supplied, returning the arguments
//   - 2026-10-02 16:18: cannot fail, the arguments arriving as a Struct
//   - 2026-10-02 16:21: everything the options tell, the logger as well
//   - 2026-10-29 01:24: exported, as Apply, so a caller applies a run's options
//     once and hands the result to Perform
func Apply(opts []RunOption) *Given {
	given := new(Given)

	for _, opt := range opts {
		opt(given)
	}

	return given
}

// Changes is the function a started run's changes go to, or nil.
//
// Revisions:
//   - 2026-10-29 01:24: initial creation
func (g *Given) Changes() func(change *workflowpb.Change) {
	return g.watch
}

// Logger is the logger a run's prints go to, or nil when the options named
// none and the prints go to slog's default.
//
// Revisions:
//   - 2026-10-29 01:24: initial creation
func (g *Given) Logger() *slog.Logger {
	return g.logger
}

// Supplied is what the run this thread initialises for supplied, and whether
// this thread is one a run bound at all.
//
// The second answer is what separates a run that supplied nothing, where every
// declaration takes its default, from a thread running a function body, where
// no value could ever arrive. Every unit a run initialises is bound, with or
// without values; a thread nobody bound is running a body.
//
// The map is nil for a bound run that supplied nothing, which reads as empty.
// It is the run's own, for reading only.
//
// Exported for the plugin that reads it, the way the scheduler exports what a
// store belongs to. A plugin holding per-run state reaches whoever owns the
// thread, and the thread a module initialises on is this package's.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-10-29 01:24: answers a nil map, bound, for a run that supplied
//     nothing, where it answered an empty one
func Supplied(thread *starlark.Thread) (map[string]*structpb.Value, bool) {
	held, ok := thread.Local(ARGS_LOCAL).(map[string]*structpb.Value)

	return held, ok
}

// _Bind puts a run's arguments on the thread a unit initialises on.
//
// A run that supplied nothing binds a nil map, not an empty one: the local
// still holds the map's type, so Supplied still answers that the thread is
// bound, and a lookup in a nil map finds nothing, as one in an empty map does.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-10-02 16:08: takes the arguments, which a run is handed rather than
//     finding on its context
//   - 2026-10-29 01:24: binds a nil map as it is, rather than making an empty
//     one for each run that supplied nothing
func _Bind(
	thread *starlark.Thread,
	supplied map[string]*structpb.Value,
) {
	thread.SetLocal(
		ARGS_LOCAL,
		supplied,
	)
}

// _Unbind takes a run's arguments off the thread its units initialised on,
// once they have.
//
// The entry point runs on that same thread afterwards, and is a body like any
// other: no declaration can be made there, and a call that tried would read
// nothing.
//
// Revisions:
//   - 2026-10-02 01:41: initial creation
func _Unbind(thread *starlark.Thread) {
	thread.SetLocal(
		ARGS_LOCAL,
		nil,
	)
}
