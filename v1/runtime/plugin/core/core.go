// Package core gives a script the builtins the runtime itself supplies: spawn,
// join, cancel, assert and sleep. Importing it is what enables it, as with any
// plugin - and the facade imports it, so a host that imports only the facade
// still gets them.
//
// The builtins live here rather than in the scheduler so the scheduler is
// machinery and nothing a script names. That also removes the cycle that used
// to make the scheduler satisfy the plugin interface without importing it and
// wait for the facade to register it.
package core

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
)

const (
	// NAME is what this plugin is called when a conflict has to name it.
	NAME = "core"

	SPAWN  = "spawn"
	JOIN   = "join"
	CANCEL = "cancel"
	ASSERT = "assert"
	SLEEP  = "sleep"
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
type _Core struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-19 22:18: initial creation, on scheduler.Builtins
//   - 2026-09-21 09:46: moved here
func (c *_Core) Name() string {
	return NAME
}

// Values returns the five names the runtime itself supplies.
//
// A fresh map on every call, because a package-level variable would be one map
// shared by every host, and a host that added a name to it would be adding it
// to everybody's environment.
//
// Revisions:
//   - 2026-09-19 22:18: initial creation, on scheduler.Builtins
//   - 2026-09-21 09:46: moved here
func (c *_Core) Values() starlark.StringDict {
	return starlark.StringDict{
		SPAWN:  starlark.NewBuiltin(SPAWN, _Spawn),
		JOIN:   starlark.NewBuiltin(JOIN, _Join),
		CANCEL: starlark.NewBuiltin(CANCEL, _Cancel),
		ASSERT: starlark.NewBuiltin(ASSERT, _Assert),
		SLEEP:  starlark.NewBuiltin(SLEEP, _Sleep),
	}
}

// Builtins returns the five names, for a test or a host that builds an
// environment without the registry.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func Builtins() starlark.StringDict {
	return (&_Core{}).Values()
}
