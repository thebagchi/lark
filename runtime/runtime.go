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
	"github.com/thebagchi/lark/runtime/artifact"
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
)

// The failures a host can act on. Each is the subpackage's own value, so
// errors.Is matches whether a host names it through this package or through
// the one that raised it.
var (
	ErrCycle       = artifact.ErrCycle
	ErrNoGlobal    = artifact.ErrNoGlobal
	ErrNoLoader    = artifact.ErrNoLoader
	ErrNoMain      = artifact.ErrNoMain
	ErrNoUnit      = artifact.ErrNoUnit
	ErrNotCallable = artifact.ErrNotCallable
	ErrAssert      = scheduler.ErrAssert
	ErrCancelled   = scheduler.ErrCancelled
	ErrNotAHandle  = scheduler.ErrNotAHandle
	ErrNotAName    = scheduler.ErrNotAName
	ErrNoRun       = scheduler.ErrNoRun
	ErrConflict    = plugin.ErrConflict
)

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
