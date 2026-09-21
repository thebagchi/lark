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

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/artifact"
	"github.com/thebagchi/lark/runtime/observe"
	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/plugin/core"
	"github.com/thebagchi/lark/runtime/scheduler"
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
	Store          = observe.Store
	Workflow       = workflowpb.Workflow
	Graph          = workflowpb.Graph
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
	ErrUnknown       = observe.ErrUnknown
)

// STORE is the runtime's own store of running scripts, which is what makes
// Status a call rather than a method on something a host has to hold.
//
// It is package-level state, and that costs two things worth knowing. Two hosts
// in one process share it: ids cannot collide, but a run started by one is
// visible to the other. And tests in one binary cannot isolate from each
// other's runs.
//
// Neither is solved by hiding it, and both are solved by not being forced to
// use it: observe.New() builds a store of your own, with the same methods. This
// is a default, not the only way in.
var STORE = observe.New()

// ENTRY is the one top-level function a script a host runs must define.
const ENTRY = artifact.ENTRY

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
// once with the id that finds it again.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
//   - 2026-09-20 19:55: takes options, so a graph can be supplied without
//     leaving the facade
func Start(ctx context.Context, art *Artifact, opts ...observe.Option) string {
	return STORE.Start(ctx, art, opts...)
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

// Status returns how the run with this id is doing, and forgets it if that
// answer is final.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func Status(id string) (*Workflow, error) {
	return STORE.Status(id)
}

// Wait blocks until the run with this id is over and returns what it produced.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func Wait(ctx context.Context, id string) (starlark.Value, error) {
	return STORE.Wait(ctx, id)
}

// Cancel stops the run with this id, without waiting for it.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func Cancel(id string) error {
	return STORE.Cancel(id)
}
