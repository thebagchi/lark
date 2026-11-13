// Package args gives a script the arguments its run was handed. It is one of
// the runtime's own plugins, so every script has it.
//
// A script declares one at module level and binds it to a name of its own
// choosing:
//
//	host = arg("host", "localhost")
//
// The two names need not agree, so a flow carries both. What a run is handed -
// by WithArgs, an option of the run - is bound by whoever initialises the
// module, which is why this reads the thread rather than holding anything
// itself.
//
// Revisions:
//   - 2026-09-22 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package args

import (
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/args/internal/declare"
	"github.com/thebagchi/lark/v1/runtime/plugin/args/internal/spec"
)

var (
	// ErrRequired is declare.ErrRequired.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	//   - 2026-11-10 17:15: renamed from ErrNotSupplied (C121, Q349)
	ErrRequired = declare.ErrRequired

	// ErrDeclare is declare.ErrDeclare.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	//   - 2026-11-10 17:15: renamed from ErrNotDeclaring (C121, Q349)
	ErrDeclare = declare.ErrDeclare

	// ErrValue is declare.ErrValue.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	//   - 2026-11-10 17:15: renamed from ErrNotValue (C121, Q349)
	ErrValue = declare.ErrValue
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func init() {
	plugin.Register(new(_Args))
}

// _Args is the plugin. Empty: what a run supplied belongs to that run and is
// found on the thread, so there is nothing here to hold.
//
// Revisions:
//   - 2026-09-22 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Args struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (a *_Args) Name() string {
	return spec.NAME
}

// Values returns the declaration builtin.
//
// The name is the graph package's, because that package reads a declaration
// out of a syntax tree and this one writes the builtin the tree names. One
// spelling, in the package that cannot import the other.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
//   - 2026-11-10 11:32: uses declare.Declare, which moved to internal/declare (C120b, Q315)
func (a *_Args) Values() starlark.StringDict {
	return starlark.StringDict{
		graph.ARG: starlark.NewBuiltin(
			graph.ARG,
			declare.Declare,
		),
	}
}

// Check refuses a use of arg that is not a call of it, such as b = arg. A
// declaration is then always an arg() call at its place, which is what a run
// and the lark command read to check -args keys (Q301).
//
// This is a Checking, which the compiler asks every plugin for. It does not
// ask about a file that has taken the name arg for itself.
//
// Returns ErrDeclare.
//
// Revisions:
//   - 2026-11-10 11:52: initial creation (C118, B251, Q301)
//   - 2026-11-10 17:15: uses ErrDeclare, formerly ErrNotDeclaring (C121, Q349)
func (a *_Args) Check(tree *syntax.File) error {
	return declare.Aliased(tree)
}
