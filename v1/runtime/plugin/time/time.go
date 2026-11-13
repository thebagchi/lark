// Package time gives a script the time module. Importing it is what enables it.
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package time

import (
	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/time/internal/spec"
)

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
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Module struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (m *_Module) Name() string {
	return spec.NAME
}

// Values returns the one name this plugin supplies.
//
// The module comes from go.starlark.net rather than being written here: it is
// the library's own, it is already a dependency, and a second implementation in
// one process is two behaviours a script could tell apart.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (m *_Module) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: starlarktime.Module,
	}
}
