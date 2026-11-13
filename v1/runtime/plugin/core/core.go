// Package core gives a script the builtins the runtime itself supplies: spawn,
// join, outcomes, cancel, assert and sleep. Importing it is what enables it,
// as with any plugin - and the facade imports it, so a host that imports only
// the facade still gets them.
//
// The builtins live here rather than in the scheduler so the scheduler is
// machinery and nothing a script names. That also removes the cycle that used
// to make the scheduler satisfy the plugin interface without importing it and
// wait for the facade to register it.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package core

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/spec"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func init() {
	plugin.Register(&_Core{})
}

// _Core is the plugin. Empty: what it contributes is built on each call, so
// there is nothing to hold.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Core struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-19 22:18: initial creation, on scheduler.Builtins
//   - 2026-09-21 09:46: moved here
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
func (c *_Core) Name() string {
	return spec.NAME
}

// Values returns the names the runtime itself supplies.
//
// A fresh map on every call, because a package-level variable would be one map
// shared by every host, and a host that added a name to it would be adding it
// to everybody's environment.
//
// Revisions:
//   - 2026-09-19 22:18: initial creation, on scheduler.Builtins
//   - 2026-09-21 09:46: moved here
//   - 2026-11-02 16:28: supplies outcomes (C28)
//   - 2026-11-03 00:31: supplies str, repr and print, guarded (C47)
//   - 2026-11-03 06:29: supplies fail, guarded (C47, Q61)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 11:32: uses builtin.Lined, builtin.Waited, builtin.Recorded and
//     builtin.Assert, which moved to internal/builtin (C120b, Q315)
func (c *_Core) Values() starlark.StringDict {
	return starlark.StringDict{
		common.SPAWN: starlark.NewBuiltin(
			common.SPAWN,
			_Spawn,
		),
		common.JOIN: starlark.NewBuiltin(
			common.JOIN,
			builtin.Lined(builtin.Waited),
		),
		spec.OUTCOMES: starlark.NewBuiltin(
			spec.OUTCOMES,
			builtin.Lined(builtin.Recorded),
		),
		common.CANCEL: starlark.NewBuiltin(
			common.CANCEL,
			_Cancel,
		),
		spec.ASSERT: starlark.NewBuiltin(
			spec.ASSERT,
			builtin.Assert,
		),
		common.SLEEP: starlark.NewBuiltin(
			common.SLEEP,
			_Sleep,
		),
		spec.STR:   _Shown(spec.STR),
		spec.REPR:  _Shown(spec.REPR),
		spec.PRINT: _Shown(spec.PRINT),
		spec.FAIL:  _Shown(spec.FAIL),
	}
}

// Builtins returns the six names, for a test or a host that builds an
// environment without the registry.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-11-02 16:28: includes outcomes (C28)
func Builtins() starlark.StringDict {
	return (&_Core{}).Values()
}
