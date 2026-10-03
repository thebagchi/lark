// Package runtime compiles Starlark scripts and runs them concurrently.
//
// A host imports this package and nothing else. What it holds - a compiler, an
// artifact, a run - are the subpackages' own types under another name, so a
// value crosses this boundary without conversion.
//
// A run tells its host two things: its graph, which the run Start returns
// answers with Status, and what it printed, which goes to the logger
// WithLogger names. Nothing else is reported, so nothing else is paid for.
//
// Importing this package is also what gives a script every plugin of the
// runtime's own - spawn, join, cancel, assert and sleep from core, and args,
// flow, state, event, json, math, time and the rest - since it imports each of
// them, and each registers itself. They are always there. A host adds its own
// beside them with WithPlugins, and that is how a script gets the filesystem
// too: file is a plugin a host hands over, never one of the runtime's own.
package runtime

import (
	"context"
	"io"
	"log/slog"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/graph"
	"github.com/thebagchi/lark/v1/runtime/observe"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/args"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base32"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/event"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/hash"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/json"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/jsonpath"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/math"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/path"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/random"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/regexp"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/time"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/utils"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/script"
)

// The types a host holds. These are aliases rather than new types: a
// runtime.Artifact and an artifact.Artifact are the same type, so nothing has
// to be converted at this boundary and a host never holds two names for one
// thing.
//
// CompileOption and RunOption are named for what they configure, a compile and
// a run, since a bare Option on a package this wide would not say what it is
// an option of.
type (
	Artifact      = artifact.Artifact
	Loader        = script.Loader
	CompileOption = artifact.Option
	RunOption     = artifact.RunOption
	Handle        = scheduler.Handle
	Plugin        = plugin.Plugin
	Execution     = observe.Execution
	Flow          = workflowpb.Flow
	Graph         = workflowpb.Graph
	Change        = workflowpb.Change
	Source        = script.Source
)

// The failures a host can act on. Each is the subpackage's own value, so
// errors.Is matches whether a host names it through this package or through
// the one that raised it.
//
// The rule for what is here: every sentinel a call through this package can
// return. A flow's ERR_ARITY and ERR_NO_MAIN are the compiler's own values, so
// each is here once. A plugin a host chooses to import - flow, state, jsonpath
// - keeps its own, because the host already names that package.
var (
	ERR_ARITY           = artifact.ERR_ARITY
	ERR_CYCLE           = artifact.ERR_CYCLE
	ERR_NO_MAIN         = artifact.ERR_NO_MAIN
	ERR_NO_UNIT         = artifact.ERR_NO_UNIT
	ERR_NOT_JSON        = artifact.ERR_NOT_JSON
	ERR_ASSERT          = core.ERR_ASSERT
	ERR_NOT_A_CONDITION = core.ERR_NOT_A_CONDITION
	ERR_NOT_A_HANDLE    = core.ERR_NOT_A_HANDLE
	ERR_NOT_A_NAME      = core.ERR_NOT_A_NAME
	ERR_CAPTURES        = core.ERR_CAPTURES
	ERR_INTERRUPTED     = core.ERR_INTERRUPTED
	ERR_CANCELLED       = scheduler.ERR_CANCELLED
	ERR_NESTED          = scheduler.ERR_NESTED
	ERR_DURATION        = scheduler.ERR_DURATION
	ERR_MEMORY          = scheduler.ERR_MEMORY
	ERR_CONFLICT        = plugin.ERR_CONFLICT
	ERR_DUPLICATE       = graph.ERR_DUPLICATE
	ERR_ARGUMENTS       = graph.ERR_ARGUMENTS
	ERR_CONSTANT        = graph.ERR_CONSTANT
	ERR_NOT_FORKED      = graph.ERR_NOT_FORKED
	ERR_UNRESOLVED      = graph.ERR_UNRESOLVED
	ERR_NO_BODY         = graph.ERR_NO_BODY
	ERR_FORM            = graph.ERR_FORM
	ERR_NOT_CARRIED     = graph.ERR_NOT_CARRIED
	ERR_SIGNATURE       = graph.ERR_SIGNATURE
	ERR_COLLISION       = graph.ERR_COLLISION
	ERR_ALIAS           = graph.ERR_ALIAS
)

// Compile builds source, and every module it loads, into an artifact, without
// running any of it.
//
// Without options a script is given every plugin of the runtime's own. Its
// modules come from the source's own Modules, or are read as files beside the
// one that loaded them when it names none.
//
// Revisions:
//   - 2026-10-03 00:12: initial creation, replacing NewCompiler, which held a
//     loader the source now names itself
func Compile(source *Source, opts ...CompileOption) (*Artifact, error) {
	return artifact.Compile(source, opts...)
}

// Load reads back a bundle Artifact.Save wrote, as an artifact Start runs, so
// a host can compile once and run elsewhere.
//
// It takes the options the compile took, since compiled code names what it
// calls: a bundle compiled with plugins of the host's is loaded with them too.
// The runtime's own are always there, as they are for Compile.
//
// Returns ERR_NO_UNIT for a bundle that does not hold every module it loads,
// and a wrapped error for bytes that are not a bundle.
//
// Revisions:
//   - 2026-10-03 00:26: initial creation
func Load(bundle []byte, opts ...CompileOption) (*Artifact, error) {
	return artifact.Load(bundle, opts...)
}

// WithPlugins makes a compile give its script the names plugins supply, as
// well as those of every plugin of the runtime's own. It is how a host gives a
// script plugins of its own, file among them, and it takes every one of them
// at once: a second WithPlugins replaces the first.
//
// A name one of these shares with another plugin is refused when a compile
// builds its environment, naming both.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-10-02 13:12: takes the plugins themselves, so a host builds no
//     registry
//   - 2026-10-02 15:56: adds the plugins to every plugin registered by import,
//     rather than replacing them
//   - 2026-10-03 00:12: an option of a compile, there being no Compiler
//   - 2026-10-03 08:27: artifact's own, which now adds to the runtime's plugins too
func WithPlugins(plugins ...Plugin) CompileOption {
	return artifact.WithPlugins(plugins...)
}

// Derive reads source, and every module it loads, into the flow it describes.
//
// The reverse of Emit, so a user interface can show an existing script as a
// flow, edit it, and hand it back. A function the walk cannot model travels as
// its body text, so a flow is never short of what the script does, and what
// cannot travel at all is refused. Its modules come from the source, as they
// do for Compile.
//
// Returns ERR_NO_MAIN for a module, ERR_CONSTANT, ERR_NOT_CARRIED and
// ERR_SIGNATURE for what a flow cannot carry, ERR_COLLISION and ERR_ALIAS for a
// load it cannot inline, and ERR_CYCLE for modules that load each other.
//
// Revisions:
//   - 2026-10-02 23:51: initial creation
//   - 2026-10-03 00:12: returns the flow alone, refusing a cycle of loads
//     rather than reporting it beside the flow
func Derive(source *Source) (*Flow, error) {
	return graph.Of(source)
}

// Check refuses a flow that could not become the script it describes: a name
// declared twice, a call passing a function arguments it does not take or both
// kinds of argument, constants naming each other, a thread joined or cancelled
// where it was not spawned, or a name that resolves to nothing.
//
// Emit runs it before it writes anything, so a host need not; a host that wants
// a flow from a user interface refused as it arrives, before it is written,
// calls it itself.
//
// Revisions:
//   - 2026-10-02 23:31: initial creation
func Check(flow *Flow) error {
	return graph.Check(flow)
}

// Emit is the Starlark script flow describes, ready for Compile, once Check
// accepts the flow.
//
// It checks first, so a flow a user interface sent becomes a script in one
// call, and is refused with what Check returns when it could not become one.
// It does not compile what it writes, so a body that is not Starlark is a
// compile error with a line number. A statement it has no form for is
// ERR_FORM, and a function with no body is ERR_NO_BODY.
//
// Revisions:
//   - 2026-10-02 23:31: initial creation
//   - 2026-10-03 16:29: checks the flow before writing it
func Emit(flow *Flow) ([]byte, error) {
	err := graph.Check(flow)
	if err != nil {
		return nil, err
	}

	return graph.Emit(flow)
}

// Start evaluates art's entry point on a goroutine of its own and returns at
// once with the run itself: something to wait for and to ask about. It is the
// one way to run an artifact - Wait on what it returns is a run that blocks -
// so every run option means the same on every run.
//
// Cancelling ctx is the one way to stop the run, and Wait then returns
// ERR_CANCELLED.
//
// Nothing holds the run on the caller's behalf. What is worth keeping about a
// finished run, and for how long, is the host's decision - so the run is the
// caller's to hold and to let go of.
//
// Status is the run's graph, which is what a user interface draws. What the
// run prints goes to the logger its options name.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
//   - 2026-09-20 19:55: takes options, so a graph can be supplied without
//     leaving the facade
//   - 2026-09-23 23:20: returns the run rather than an id into a store, which
//     no longer exists
//   - 2026-10-02 13:12: takes no options, what a run prints being its
//     context's transcript
//   - 2026-10-02 16:08: takes the run's options, its script's arguments among
//     them
//   - 2026-10-03 00:21: the one way to run, an artifact having no Run or Invoke of its
//     own, and stopped by its context alone
func Start(ctx context.Context, art *Artifact, opts ...RunOption) *Execution {
	return observe.Start(ctx, art, opts...)
}

// Run compiles source with the runtime's own plugins, runs its main, and
// returns what main returned once the run is over: Compile, Start and Wait in
// one call, for a host that wants the result and nothing more.
//
// Every run option means here what it means to Start, which Run calls. A host
// handing a script plugins of its own compiles it with Compile and runs it with
// Start, since what a compile is given and what a run is given are not one
// list; so does a host that reads Status or stops the run itself.
//
// Returns what Compile refuses a script with, and otherwise what Wait returns.
//
// Revisions:
//   - 2026-10-03 16:31: initial creation
func Run(ctx context.Context, source *Source, opts ...RunOption) (starlark.Value, error) {
	art, err := Compile(source)
	if err != nil {
		return nil, err
	}

	return Start(ctx, art, opts...).Wait()
}

// WithArgs supplies a run's script with its arguments, each field of args the
// value of the argument the script declares under that name.
//
// A script declares one at module level - host = arg("host", "localhost") -
// and every run of one artifact may be handed different values, because a run
// initialises the script itself. The arguments are the script's, not the
// runtime's, so a run is handed them rather than finding them on its context.
//
// A map of Go values, so a host imports nothing to supply them, and JSON it
// holds is one json.Unmarshal into the map away. Each must be a value JSON can
// hold; one that is not fails the run when it starts, with ERR_NOT_JSON.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-10-02 13:12: reads the JSON object itself, absorbing Parsed
//   - 2026-10-02 16:08: an option of a run, rather than a context carrying the
//     arguments
//   - 2026-10-02 16:18: takes a Struct, a JSON object by its type, rather than
//     text to parse
//   - 2026-10-03 16:30: takes a map of Go values, the Struct built inside
func WithArgs(args map[string]any) RunOption {
	return artifact.WithArgs(args)
}

// WithLog writes what a run's script prints to w, one line a print: the time,
// the level, the thread and the function that printed it, and then the line,
// as the lark command prints them. A host with a slog handler of its own
// passes it to WithLogger instead.
//
// Revisions:
//   - 2026-10-03 16:31: initial creation
func WithLog(w io.Writer) RunOption {
	return artifact.WithLog(w)
}

// WithMemory holds a run to bytes of what this runtime allocates on its
// script's behalf - what it reads, stores, copies and spawns - rather than the
// default of 256MB. It does not bound the interpreter's own allocations. Zero
// or less is the default.
//
// Revisions:
//   - 2026-10-03 08:31: initial creation, so a host sets a ceiling through this
//     package alone
func WithMemory(bytes int64) RunOption {
	return artifact.WithMemory(bytes)
}

// WithChanges sends a started run's graph, as it changes, to watch: an option
// of Start. Each Change is the JSON Patch that takes a copy of the graph from
// before one step to after it, and a copy starts as {}, so a user interface
// patching its copy shows which function each thread is executing as it
// happens. watch is called one change at a time, in order, on the goroutine of
// the thread whose step it was; Status still answers for the whole graph.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func WithChanges(watch func(change *Change)) RunOption {
	return artifact.WithChanges(watch)
}

// WithLogger sends what a run's script prints to logger. Each printed line is
// one record - the line as the message, with the thread and the function that
// printed it as attributes - so the handler the host chose writes the time and
// the layout. Without it, the lines go to slog's default logger.
//
// Revisions:
//   - 2026-10-02 16:21: initial creation, replacing WithTranscript
func WithLogger(logger *slog.Logger) RunOption {
	return artifact.WithLogger(logger)
}
