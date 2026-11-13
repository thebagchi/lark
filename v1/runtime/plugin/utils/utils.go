// Package utils gives a script the utils module of section 10.8 of the brief,
// which holds one thing: the local time as text. Importing it is what enables
// it.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package utils

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/utils/internal/clock"
	"github.com/thebagchi/lark/v1/runtime/plugin/utils/internal/spec"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func init() {
	plugin.Register(&_Utils{})
}

// _Utils is the plugin. Empty: it reads the clock and nothing else.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Utils struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (u *_Utils) Name() string {
	return spec.NAME
}

// Values returns the utils module.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 11:32: uses clock.Datetime, which moved to internal/clock (C120b, Q315)
func (u *_Utils) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.DATETIME: starlark.NewBuiltin(
					spec.NAME+"."+spec.DATETIME,
					clock.Datetime,
				),
			},
		},
	}
}
