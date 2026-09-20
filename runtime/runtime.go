// Package runtime compiles Starlark scripts and runs them concurrently.
//
// A host imports this package and nothing else. What it holds - a compiler, an
// artifact, a handle - are the subpackages' own types under another name, so a
// value crosses this boundary without conversion.
//
// Importing this package is also what gives a script spawn, join, cancel and
// assert: the init below registers them, and it is the only place that can,
// being the only package that imports both the scheduler and the registry.
package runtime

import (
	"context"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/artifact"
	"github.com/thebagchi/lark/runtime/observe"
	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// The types a host holds. These are aliases rather than new types: a
// runtime.Artifact and an artifact.Artifact are the same type, so nothing has
// to be converted at this boundary and a host never holds two names for one
// thing.
type (
	Artifact = artifact.Artifact
	Compiler = artifact.Compiler
	Loader   = artifact.Loader
	Option   = artifact.Option
	Handle   = scheduler.Handle
	Plugin   = plugin.Plugin
	Store    = observe.Store
	Workflow = workflowpb.Workflow
	Graph    = workflowpb.Graph
)

// The failures a host can act on. Each is the subpackage's own value, so
// errors.Is matches whether a host names it through this package or through
// the one that raised it.
var (
	ErrCycle         = artifact.ErrCycle
	ErrNoGlobal      = artifact.ErrNoGlobal
	ErrNoLoader      = artifact.ErrNoLoader
	ErrNoMain        = artifact.ErrNoMain
	ErrNoUnit        = artifact.ErrNoUnit
	ErrNotCallable   = artifact.ErrNotCallable
	ErrAssert        = scheduler.ErrAssert
	ErrNotACondition = scheduler.ErrNotACondition
	ErrCancelled     = scheduler.ErrCancelled
	ErrNotAHandle    = scheduler.ErrNotAHandle
	ErrNotAName      = scheduler.ErrNotAName
	ErrNoRun         = scheduler.ErrNoRun
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

// init registers the scheduler's builtins, which is what makes spawn, join,
// cancel and assert exist for every script compiled in this process.
//
// It lives here because it can live nowhere else. The artifact package imports
// the scheduler, so the scheduler cannot import the registry without a cycle;
// it satisfies the plugin interface structurally and waits for somebody to
// install it. This package imports all three and is that somebody.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
func init() {
	plugin.Register(&scheduler.Builtins{})
}

// NewCompiler returns a Compiler configured by opts.
//
// Without options it resolves a module as a file beside the one that loaded it.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
func NewCompiler(opts ...Option) *Compiler {
	return artifact.NewCompiler(opts...)
}

// WithLoader makes a Compiler reach modules through loader.
//
// Revisions:
//   - 2026-09-19 23:29: initial creation
func WithLoader(loader Loader) Option {
	return artifact.WithLoader(loader)
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
