package artifact

import (
	"errors"
	"fmt"
	"io"
	"log/slog"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
)

// ARGS_LOCAL is what a run's arguments travel under on the thread a unit
// initialises on. A thread local is keyed by a string, so this is one.
//
// Revisions:
//   - 2026-09-22 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-12 17:20: is script.ARGS_LOCAL, so the scheduler can read it too (C128, B278)
const (
	ARGS_LOCAL = script.ARGS_LOCAL
)

// ErrJSON is returned for a run handed arguments holding a value JSON
// cannot: a function, a channel, a struct. Arguments are values, which is the
// limit a flow carries: null, bool, int, float, valid UTF-8 text, bytes, list and
// map with string keys. An int outside int64, text that is not valid UTF-8 and a
// number that is no number are ErrJSON too.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: renamed from ErrNotJSON (C121, Q348)
//   - 2026-11-13 12:49: the kinds of a lark Value, the int in int64 (C132, Q437, Q438)
var (
	ErrJSON = errors.New("an argument holds a value JSON cannot")
)

// WithArgs supplies a run's script with its arguments: each entry of args is
// the value of the argument the script declares under that name.
//
// The script's, not the runtime's, so a run is handed them rather than finding
// them on its context. Each run of one artifact may be handed different ones,
// because each run initialises the script again. A map of Go values, which is
// what a host holds - JSON it has read with encoding/json included - and each
// must be one a lark Value can hold, which is the limit a flow carries: a slice or
// array of them is a list, a map with a string key kind is a map, a named
// basic type is its value, and a pointer is what it points to (nil is null).
// The rules are those of protobuf JSON, strictly: an integer type, and a
// json.Number written as an integer, is an exact int, and one outside int64 is
// out of range; a float type, and a json.Number with a fraction or an exponent,
// is a float; a string is text and must be valid UTF-8, and is never read as
// base64; a []byte is bytes, the one way a host gives bytes. One that is not
// fails the run when it starts, with ErrJSON, since an option has nowhere
// to say so.
//
// A key that no arg() of the script declares fails the run, once the modules
// have initialised and before main is called, with spelling.ErrArgs, naming the key and the
// declared name nearest to it when one is close. A typo is then an error, not a default taken
// in silence. The names are those of the merged script, so an artifact loaded
// from a bundle is checked too.
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
//   - 2026-11-05 16:58: takes typed slices, typed maps, named basic types and
//     pointers too, converting them (C85, Q167)
//   - 2026-11-05 19:40: a key no arg() declares fails the run with ErrArity
//     (C71, Q170)
//   - 2026-11-07 23:04: uses common.RunOption and common.Given, moved from here (C91w6)
//   - 2026-11-08 10:35: joins its refusal to an earlier one, so WithLog beside WithLogger
//     is not lost (C75, Q235)
//   - 2026-11-08 11:20: a second WithArgs is refused (C75, Q256)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
//   - 2026-11-08 12:50: a refused map counts as a call, so a second one is named; the marker is
//     built once, outside the closure (C75, Q256)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 17:15: uses ErrJSON, formerly ErrNotJSON (C121, Q348)
//   - 2026-11-13 12:49: the arguments are lark Values read by the strict rules of protobuf
//     JSON, not a google.protobuf.Struct (C132, Q437, Q438)
//   - 2026-11-13 23:55: uses spelling.ErrArgs, formerly ErrArity (C136, Q475)
func WithArgs(args map[string]any) common.RunOption {
	held, err := _Map(args)
	if err != nil {
		err = fmt.Errorf(
			"%w: %w",
			ErrJSON,
			err,
		)
	}

	// A refused map still marks the option as given, so a repeat is a repeat
	// whatever the first call held; the run never reads Args then. Made here,
	// once: the option is applied by many runs, and only reads what it holds.
	if held == nil {
		held = new(workflowpb.Map)
	}

	return func(given *common.Given) {
		if args == nil {
			return
		}

		if given.Args != nil {
			_Twice(
				given,
				"WithArgs",
			)

			return
		}

		given.Args = held
		given.Refused = errors.Join(
			given.Refused,
			err,
		)
	}
}

// _Map is args as the Map a run is handed, once the typed slices, typed maps,
// named basic types and pointers in it are converted to plain values (C85), and
// each plain value to the lark Value of its kind.
//
// Revisions:
//   - 2026-11-05 16:58: initial creation (C85)
//   - 2026-11-13 12:49: renamed from _Struct, and writes a workflow.Map (C132, Q437)
func _Map(args map[string]any) (*workflowpb.Map, error) {
	plain, err := _Converted(args)
	if err != nil {
		return nil, err
	}

	return _Valued(plain)
}

// WithLog writes what a run's script prints to w, one line a print: the time,
// the level, the thread and the function that printed it, and then the line,
// as the lark command prints them.
//
// The one way to keep a run's output (Q340): a host with a writer gives it
// here, and one that wants records reads the lines it wrote. The message is
// last, so a line says where it came from before what was said, which slog's
// own handlers do the other way round.
//
// A second WithLog is refused (Q256). A nil writer is not given: it is no call
// (Q261).
//
// Revisions:
//   - 2026-10-03 16:31: initial creation
//   - 2026-11-07 23:04: uses common.RunOption and common.Given, moved from here (C91w6)
//   - 2026-11-08 10:35: refuses WithLog after WithLogger (C75, Q235)
//   - 2026-11-08 11:20: refuses a second WithLog too (C75, Q256)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
//   - 2026-11-10 17:15: the only log option, since WithLogger is dropped; nothing to refuse it
//     beside (C121, Q340)
func WithLog(w io.Writer) common.RunOption {
	logger := slog.New(_NewTrailing(w))

	return func(given *common.Given) {
		switch {
		case w == nil:
			return
		case given.Logger != nil:
			_Twice(
				given,
				"WithLog",
			)

			return
		}

		given.Logger = logger
	}
}

// _Twice adds to given.Refused the refusal of option given a second time. A
// plain error naming the option, with the hint in parentheses, and no sentinel
// (Q256, Q255, Q158, Q180).
//
// Revisions:
//   - 2026-11-08 11:20: initial creation (C75, Q256)
func _Twice(
	given *common.Given,
	option string,
) {
	given.Refused = errors.Join(
		given.Refused,
		fmt.Errorf(
			"%s is given twice (give it once)",
			option,
		),
	)
}

// WithMemoryWatch turns on the memory watch for a run. It is off by default.
//
// The watch reads the memory in use on the machine and in this process's
// cgroup, counting any process's use, and when the tighter share reaches 95
// percent it ends the run the way assert does: Wait returns an error that
// wraps memwatch.ErrLimit, "memory limit reached". A system with no
// /proc/meminfo or no cgroup v2 refuses the run with memwatch.ErrUnavailable,
// naming what it could not read, before anything runs.
//
// abandon, when not nil, is called with that error if the run has not ended
// half a second after the trip, which is what happens to a run stuck in a Go
// builtin that never looks at its context. The lark command passes a function
// that prints one line and exits 5. Nil leaves the process alone and keeps the
// policy with the host. A second abandon is refused with a second
// WithMemoryWatch; a nil abandon is not given, so it is no call, and the watch
// is on.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-02 23:47: the exit code in this text is 5 (C27)
//   - 2026-11-07 23:04: uses common.RunOption and common.Given, moved from here (C91w6)
//   - 2026-11-08 11:20: a second WithMemoryWatch is refused (C75, Q256)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func WithMemoryWatch(abandon func(error)) common.RunOption {
	return func(given *common.Given) {
		// The watch is on whatever abandon is; a nil abandon is not a value
		// (Q261), so it neither counts as a call nor undoes an earlier one.
		if given.Memory == nil {
			given.Memory = new(common.MemoryWatch)
		}

		if abandon == nil {
			return
		}

		if given.Memory.Abandon != nil {
			_Twice(
				given,
				"WithMemoryWatch",
			)

			return
		}

		given.Memory.Abandon = abandon
	}
}

// Reporting has a run tell into what it does: each line that starts and ends.
//
// For observe, which keeps a started run's graph, and for a test watching a
// run's lines; a host watches through WithChanges instead.
//
// Revisions:
//   - 2026-10-03 08:31: initial creation, replacing the reporter a context carried
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 23:04: uses common.RunOption and common.Given, moved from here (C91w6)
func Reporting(into common.Reporter) common.RunOption {
	return func(given *common.Given) {
		given.Into = into
	}
}

// WithChanges sends a started run's graph, as it changes, to watch: one Change
// a step, the JSON Patch that takes a copy of the graph from before the step to
// after it, from the first change, which makes an empty graph running.
//
// watch is called on the goroutine of the thread whose step it was, one change
// at a time and in the order they were made, and no other step is folded while
// it runs, so a slow one holds the run up. Asked from inside, the run's whole
// graph is exactly the graph after the change being told. A second WithChanges
// is refused; a nil watch is not given, and is no call (Q261).
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-07 23:04: uses common.RunOption and common.Given, moved from here (C91w6)
//   - 2026-11-08 11:20: a second WithChanges is refused (C75, Q256)
//   - 2026-11-08 12:20: a nil value is not given, and is no call (C75, Q261)
func WithChanges(watch func(change *workflowpb.Change)) common.RunOption {
	return func(given *common.Given) {
		if watch == nil {
			return
		}

		if given.Watch != nil {
			_Twice(
				given,
				"WithChanges",
			)

			return
		}

		given.Watch = watch
	}
}

// WithTracking asks for the graph of a run to be kept as the run goes, so
// Status answers at any moment with the functions called so far.
//
// Without it, and without WithChanges, which asks for the same, a run keeps its
// graph for the end: Status answers an empty running graph until the run ends,
// and the graph of the whole run after it - the same graph either way. The
// live graph takes a lock that every thread of the run waits for at each call,
// so a host that reads the graph only at the end does better to leave it off.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation, from the research POC q1_endgraph
//   - 2026-11-07 23:04: uses common.RunOption and common.Given, moved from here (C91w6)
//   - 2026-11-10 22:40: renamed from WithLiveGraph (C121, Q350)
func WithTracking() common.RunOption {
	return func(given *common.Given) {
		given.Live = true
	}
}

// Apply is what opts tell a run, each applied in order. Two that set one thing
// are refused: the second adds its refusal to Given.Refused (Q256).
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
//   - 2026-11-04 23:50: skips a nil option (C86)
//   - 2026-11-07 23:04: uses common.RunOption and common.Given, moved from here (C91w6)
//   - 2026-11-08 11:20: the text says a repeat is refused, not that the later wins (C75, Q256)
func Apply(opts []common.RunOption) *common.Given {
	given := new(common.Given)

	for _, opt := range opts {
		if opt == nil {
			continue
		}

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
//   - 2026-11-04 23:50: a nil thread is bound to nothing (C86 fix round 2)
//   - 2026-11-13 12:49: the values are workflow.Value (C132, Q437)
func Supplied(thread *starlark.Thread) (map[string]*workflowpb.Value, bool) {
	if thread == nil {
		return nil, false
	}

	held, ok := thread.Local(ARGS_LOCAL).(map[string]*workflowpb.Value)

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
//   - 2026-11-13 12:49: the values are workflow.Value (C132, Q437)
func _Bind(
	thread *starlark.Thread,
	supplied map[string]*workflowpb.Value,
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
