// Package runtime compiles Starlark scripts and runs them concurrently.
//
// A host imports this package and nothing else. What it holds - a compiler, an
// artifact, a handle - are the subpackages' own types under another name, so a
// value crosses this boundary without conversion.
//
// Importing this package is also what gives a script spawn, join, cancel,
// assert and sleep: it imports the core plugin, which registers them the way
// every plugin registers itself.
package runtime

import (
	"context"

	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/observe"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// The types a host holds. These are aliases rather than new types: a
// runtime.Artifact and an artifact.Artifact are the same type, so nothing has
// to be converted at this boundary and a host never holds two names for one
// thing.
//
// CompilerOption is named for what it configures, because observe has an
// Option of its own and a bare Option here would claim to be the only one.
type (
	Artifact       = artifact.Artifact
	Compiler       = artifact.Compiler
	Loader         = artifact.Loader
	CompilerOption = artifact.Option
	Handle         = scheduler.Handle
	Reporter       = scheduler.Reporter
	Plugin         = plugin.Plugin
	Registry       = plugin.Registry
	Execution      = observe.Execution
	Log            = observe.Log
	Workflow       = workflowpb.Workflow
	Graph          = workflowpb.Graph
	Change         = workflowpb.Change
	Watcher        = observe.Watcher
)

// The failures a host can act on. Each is the subpackage's own value, so
// errors.Is matches whether a host names it through this package or through
// the one that raised it.
//
// The rule for what is here: every sentinel of every package this file
// imports. A plugin a host chooses to import - flow, state, jsonpath - keeps
// its own, because the host already names that package.
var (
	ErrCycle         = artifact.ErrCycle
	ErrNoGlobal      = artifact.ErrNoGlobal
	ErrNoLoader      = artifact.ErrNoLoader
	ErrNoMain        = artifact.ErrNoMain
	ErrNoUnit        = artifact.ErrNoUnit
	ErrNotCallable   = artifact.ErrNotCallable
	ErrNotObject     = artifact.ErrNotObject
	ErrAssert        = core.ErrAssert
	ErrNotACondition = core.ErrNotACondition
	ErrNotAHandle    = core.ErrNotAHandle
	ErrNotAName      = core.ErrNotAName
	ErrInterrupted   = core.ErrInterrupted
	ErrCancelled     = scheduler.ErrCancelled
	ErrNoRun         = scheduler.ErrNoRun
	ErrNotLocal      = scheduler.ErrNotLocal
	ErrNested        = scheduler.ErrNested
	ErrDuration      = scheduler.ErrDuration
	ErrConflict      = plugin.ErrConflict
)

const (
	// ENTRY is the one top-level function a script a host runs must define.
	ENTRY = artifact.ENTRY

	// LOG_SUFFIX is what a run's own file is called after the run or the
	// script it belongs to.
	LOG_SUFFIX = observe.SUFFIX
)

// NewCompiler returns a Compiler configured by opts.
//
// Without options it resolves a module as a file beside the one that loaded
// it, and gives a script every plugin registered by import.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
//   - 2026-09-21 09:46: the option type is named for what it configures
func NewCompiler(opts ...CompilerOption) *Compiler {
	return artifact.NewCompiler(opts...)
}

// WithLoader makes a Compiler reach modules through loader.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
func WithLoader(loader Loader) CompilerOption {
	return artifact.WithLoader(loader)
}

// WithAuthored makes a Compiler carry graph rather than deriving one, for the
// case where the graph came first and the script was generated from it.
//
// Not WithGraph, which tells a run what its script could do. This says what a
// bundle carries.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func WithAuthored(graph *Graph) CompilerOption {
	return artifact.WithAuthored(graph)
}

// Pending is a graph as a workflow that has not started, which is what a user
// interface draws from a bundle before anything runs.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func Pending(graph *Graph) *Workflow {
	return observe.Pending(graph)
}

// WithPlugins makes a Compiler give its scripts the names in registry rather
// than those every plugin registered by import.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func WithPlugins(registry *Registry) CompilerOption {
	return artifact.WithPlugins(registry)
}

// NewRegistry returns a registry holding no plugins, for a host that wants a
// compiler to see a set of its own.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func NewRegistry() *Registry {
	return plugin.New()
}

// Register installs a plugin of the host's own, adding its names to every
// script compiled in this process.
//
// Returns nothing: it is meant to be called from an init, which cannot handle
// an error, and CLAUDE.md forbids ignoring one. A name that clashes with one
// already supplied is refused when a compile builds its environment, naming
// both plugins.
//
// Revisions:
//   - 2026-09-19 23:30: initial creation
func Register(installed Plugin) {
	plugin.Register(installed)
}

// Start evaluates art's entry point on a goroutine of its own and returns at
// once with the run itself: something to stop, to wait for, and to ask about.
//
// Nothing holds the run on the caller's behalf. What is worth keeping about a
// finished run, and for how long, is the host's decision - so the run is the
// caller's to hold and to let go of.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
//   - 2026-09-20 19:55: takes options, so a graph can be supplied without
//     leaving the facade
//   - 2026-09-23 23:20: returns the run rather than an id into a store, which
//     no longer exists
func Start(ctx context.Context, art *Artifact, opts ...observe.Option) *Execution {
	return observe.Start(ctx, art, opts...)
}

// WithGraph tells a run what its script could do, so a report can say what has
// not happened yet.
//
// Here as well as on observe, because a host that imports only this package
// could otherwise not reach it - and a feature reachable only by abandoning the
// facade is one the facade does not have.
//
// observe.Option is not aliased. runtime.Option is already artifact.Option, for
// NewCompiler, and a host passing this inline never has to name the type.
//
// Revisions:
//   - 2026-09-20 19:55: initial creation
func WithGraph(graph *Graph) observe.Option {
	return observe.WithGraph(graph)
}

// WithArgs returns a context carrying what a run supplies its script's
// arguments with.
//
// A script declares one at module level - host = arg("host", "localhost") -
// and every run of one artifact may supply different values, because a run
// initialises the script itself.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func WithArgs(ctx context.Context, supplied map[string]*structpb.Value) context.Context {
	return artifact.WithArgs(ctx, supplied)
}

// Parsed is the arguments a JSON object states, ready for WithArgs.
//
// Returns ErrNotObject for anything else, and a wrapped error for JSON that
// does not parse.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func Parsed(src []byte) (map[string]*structpb.Value, error) {
	return artifact.Parsed(src)
}

// WithWatcher returns a context carrying what a run tells its host every time
// a function changes status.
//
// A change costs nothing to deliver. The whole run, which the watcher is
// handed as a function, costs time proportional to how wide the run is - so a
// host pays for the picture only where it asks for one.
//
// Here as well as on observe for the reason WithGraph is: a feature reachable
// only by abandoning the facade is one the facade does not have.
//
// Revisions:
//   - 2026-09-23 22:48: initial creation
func WithWatcher(ctx context.Context, into Watcher) context.Context {
	return WithReporter(ctx, observe.Watching(into))
}

// WithReporter returns a context carrying what a run tells its host: every
// function starting and ending, and every line printed.
//
// Here as well as on scheduler for the reason WithGraph is: a feature
// reachable only by abandoning the facade is one the facade does not have.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func WithReporter(ctx context.Context, into Reporter) context.Context {
	return scheduler.WithReporter(ctx, into)
}

// WithPrinter returns a context carrying a reporter that only prints, for a
// host that wants a script's output and nothing else.
//
// One reporter per run: this and WithReporter set the same thing. Without
// either the interpreter's default stands, which writes to standard error.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-09-21 09:46: a reporter that prints, since a run tells its host one
//     thing
func WithPrinter(ctx context.Context, print func(string)) context.Context {
	return scheduler.WithPrinter(ctx, print)
}

// WithLog is the file a started run writes its transcript to, each line behind
// the thread that printed it.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation, as WithLogs, taking a directory
//   - 2026-09-23 23:20: takes the file, since nothing mints a name to call one
//     after
func WithLog(path string) observe.Option {
	return observe.WithLog(path)
}

// NewLog opens a file for one run's printed output, for a host that reports
// for itself rather than starting a run through this package.
//
// It is a Reporter that hears only the printing, so a host wanting the lines
// somewhere else as well writes a reporter of its own holding one of these.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func NewLog(path string) (*Log, error) {
	return observe.NewLog(path)
}
