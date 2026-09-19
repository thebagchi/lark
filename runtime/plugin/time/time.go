// Package time gives a script the time module. Importing it is what enables it.
package time

import (
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/plugin"
)

const NAME = "time"

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
func init() {
	plugin.Register(&_Module{})
}

// _Module is the plugin. Empty: what it contributes is a value the library
// already builds, so there is nothing to hold.
type _Module struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
func (m *_Module) Name() string {
	return NAME
}

// Values returns the one name this plugin supplies.
//
// The module comes from go.starlark.net rather than being written here: it is
// the library's own, it is already a dependency, and a second implementation in
// one process is two behaviours a script could tell apart.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
func (m *_Module) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: starlarktime.Module,
	}
}
