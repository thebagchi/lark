// Package plugin is how names reach the environment a script is given.
//
// The runtime's own plugins register themselves from their init, so importing
// one is the whole of enabling it, and every compile starts from what Registered
// returns. A host never registers one: it hands a compile its own plugins with
// WithPlugins, which adds them to that compile alone. The behavior lives in
// plugin/internal/registry; this package forwards to it.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:16: thin forwards to plugin/internal/registry (C91w1)
package plugin

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/registry"
)

// ErrConflict is returned when two plugins supply one name.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:16: an alias of registry.ErrConflict (C91w1)
var (
	ErrConflict = registry.ErrConflict
)

// Register adds plugin to the registered list, and is meant to be called from
// the init of one of the runtime's own plugins, so that importing that package
// is what enables it. A host's plugins go to a compile with WithPlugins
// instead. It returns nothing on purpose: a conflict is found when an
// environment is built, by Environment.
//
// Revisions:
//   - 2026-09-19 21:04: initial creation
//   - 2026-11-04 22:35: Register alone decides the plugin is cached (Q147)
//   - 2026-11-07 18:16: forwards to registry.Register; the earlier revisions are there (C91w1)
func Register(plugin common.Plugin) {
	registry.Register(plugin)
}

// Environment merges the names every plugin supplies into one environment, in
// the order the plugins are given.
//
// Returns ErrConflict naming both plugins and the name they clash on.
//
// Revisions:
//   - 2026-09-19 21:06: initial creation
//   - 2026-11-07 18:16: forwards to registry.Environment; the earlier revisions are there (C91w1)
func Environment(plugins []common.Plugin) (starlark.StringDict, error) {
	return registry.Environment(plugins)
}

// Registered is a copy of the list of the runtime's own plugins, in the order
// their inits registered them. The caller owns the copy and may append to it.
//
// Revisions:
//   - 2026-11-07 18:16: initial creation, in place of the exported var DEFAULT (Q221, C91w1)
func Registered() []common.Plugin {
	return registry.Registered()
}

// NewBuiltin is starlark.NewBuiltin with a check of the arguments in front of
// fn. The body fn has the shape starlark.NewBuiltin takes: the thread, the
// builtin being called, its positional arguments and its keyword pairs.
//
// A host plugin author should build each builtin with it whenever fn reads its
// arguments with starlark.UnpackArgs and a Go caller may call the builtin
// directly. The error text holds the builtin's name and a position, and never
// the argument.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93), public by decision (Q143)
//   - 2026-11-07 18:16: forwards to registry.NewBuiltin; BuiltinFunc dropped (Q222, Q223, C91w1)
func NewBuiltin(
	name string,
	fn func(
		thread *starlark.Thread,
		self *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error),
) *starlark.Builtin {
	return registry.NewBuiltin(
		name,
		fn,
	)
}
