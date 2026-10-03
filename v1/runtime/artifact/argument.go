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
const ARGS_LOCAL = "artifact.args"

// ERR_NOT_JSON is returned for a run handed arguments holding a value JSON
// cannot: a function, a channel, a struct. Arguments are values, which is the
// limit a flow carries.
var ERR_NOT_JSON = errors.New("an argument holds a value JSON cannot")

// RunOption sets one thing on a run of an artifact: the arguments its script
// is handed, the logger what it prints goes to, the memory it may hold, or
// where its changes go.
//
// An option rather than a parameter of Run, so a run can be told more without
// every caller changing. What travels on the context is only when to stop.
type RunOption func(given *_Given)

// _Given is what a run's options told it: the arguments its script is supplied
// with, the logger what it prints goes to, the memory it may hold, the
// function its graph's changes go to, and what watches it, each zero when
// nothing was. refused is why the arguments could not be supplied, which an
// option cannot return and the run does, before anything runs.
type _Given struct {
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
// fails the run when it starts, with ERR_NOT_JSON, since an option has nowhere
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
		err = fmt.Errorf("%w: %w", ERR_NOT_JSON, err)
	}

	return func(given *_Given) {
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
	return func(given *_Given) {
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
	return func(given *_Given) {
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
	return func(given *_Given) {
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
	return func(given *_Given) {
		given.watch = watch
	}
}

// Changes is the function opts send a started run's changes to, or nil.
//
// Exported for observe, which keeps a started run's graph and is what sends
// them; a run made here keeps none.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func Changes(opts []RunOption) func(change *workflowpb.Change) {
	return _Applied(opts).watch
}

// _Applied is what opts tell a run.
//
// Revisions:
//   - 2026-10-02 16:08: initial creation, as _Supplied, returning the arguments
//   - 2026-10-02 16:18: cannot fail, the arguments arriving as a Struct
//   - 2026-10-02 16:21: everything the options tell, the logger as well
func _Applied(opts []RunOption) *_Given {
	given := new(_Given)

	for _, opt := range opts {
		opt(given)
	}

	return given
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
//   - 2026-10-02 16:08: takes the arguments, which a run is handed rather than
//     finding on its context
func _Bind(thread *starlark.Thread, supplied map[string]*structpb.Value) {
	if supplied == nil {
		supplied = map[string]*structpb.Value{}
	}

	thread.SetLocal(ARGS_LOCAL, supplied)
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
	thread.SetLocal(ARGS_LOCAL, nil)
}
