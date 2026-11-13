package registry

import (
	"errors"
	"fmt"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrConflict is returned when two plugins supply one name.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:16: moved to package registry (C91w1)
var (
	ErrConflict = errors.New("two plugins supply one name")
)

// DEFAULT is every plugin of the runtime's own, in the order their inits
// registered them, and what every compile starts from.
//
// A list rather than a type holding one: it is filled once, by inits, which run
// one at a time before anything reads it, so there is nothing to guard. A
// caller gets a copy from Registered, since appending to a shared list writes
// into whatever spare room the next caller sees too.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:16: moved to package registry; Registered copies it (Q221, C91w1)
var (
	DEFAULT []common.Plugin
)

// Registered is a copy of the registered list, in registration order. The
// caller owns the copy and may append to it.
//
// Revisions:
//   - 2026-11-07 18:16: initial creation (Q221, C91w1)
func Registered() []common.Plugin {
	return slices.Clone(DEFAULT)
}

// Register adds plugin to the registered list, and is meant to be called from the init of
// one of the runtime's own plugins, so that importing that package is what
// enables it. A host's plugins go to a compile with WithPlugins instead.
//
// It returns nothing on purpose. An init cannot handle an error, and CLAUDE.md
// forbids ignoring one, so a conflict is recorded rather than raised: it is
// found when an environment is built, by Environment, which has a caller that
// can act on it.
//
// Revisions:
//   - 2026-09-19 21:04: initial creation
//   - 2026-09-21 09:46: reaches DEFAULT
//   - 2026-10-03 08:27: appends to DEFAULT, a list rather than a registry
//   - 2026-11-04 22:35: DEFAULT holds the plugin with its builtins checked, so
//     a Go caller of Values is guarded too (Q134)
//   - 2026-11-04 22:35: the names of the wrapper are built once, on first use (Q142)
//   - 2026-11-04 22:40: Register alone decides the plugin is cached (Q147)
//   - 2026-11-04 23:56: registers nothing for a nil plugin, which the wrapper would
//     otherwise hold and panic on at the first Environment (C86 rebase)
//   - 2026-11-07 18:16: moved to package registry (C91w1)
func Register(plugin common.Plugin) {
	if plugin == nil {
		return
	}

	DEFAULT = append(
		DEFAULT,
		&_Guarding{
			inner:  plugin,
			cached: true,
		},
	)
}

// Environment merges the names every plugin supplies into one environment, in
// the order the plugins are given.
//
// Returns ErrConflict naming both plugins and the name they clash on. A script's
// environment is decided before anything runs, so a clash is refused there
// rather than resolved silently - a shadowed builtin is a defect that surfaces
// much later, as wrong behaviour, somewhere else.
//
// Revisions:
//   - 2026-09-19 21:06: initial creation
//   - 2026-09-21 09:46: a method on the registry
//   - 2026-10-03 08:27: a function of the plugins given, there being no registry
//   - 2026-11-04 22:35: every builtin is checked for a nil argument and a
//     malformed keyword pair, so a Go caller gets an error, not a panic (C93)
//   - 2026-11-04 22:35: wraps only a plugin Register did not, so a builtin is
//     wrapped once (Q134)
//   - 2026-11-04 22:35: reads a registered plugin's shared dict, with no copy (Q142)
//   - 2026-11-04 22:40: wraps a host's plugin with no flag (Q147)
//   - 2026-11-04 23:56: skips a nil plugin; artifact.WithPlugins skips a
//     typed-nil one before it gets here (C86 fix round 2)
//   - 2026-11-05 16:55: returns an error, not a panic, for a plugin whose Values
//     panics (B123, C96)
//   - 2026-11-05 17:40: reads the name through _Label, which a panic of Name cannot break
//     (B123, C96 fix round 1)
//   - 2026-11-07 18:16: moved to package registry (C91w1)
//   - 2026-11-10 01:00: now uses Wrapped, were _Wrapped (C120a, Q314)
//   - 2026-11-10 16:05: quotes the clashing name through unpack.QuotePrefix and the plugin names
//     are cut (C117, B239, Q4)
func Environment(plugins []common.Plugin) (starlark.StringDict, error) {
	var (
		merged = starlark.StringDict{}
		owner  = map[string]string{}
	)

	for _, given := range plugins {
		if given == nil {
			continue
		}

		var (
			plugin     = Wrapped(given)
			names, err = plugin._Shared()
		)
		if err != nil {
			return nil, err
		}

		// In a fixed order, Keys being sorted: map iteration is randomised, and
		// the same clash would otherwise name a different name on each run.
		for _, key := range names.Keys() {
			first, taken := owner[key]
			if taken {
				return nil, fmt.Errorf(
					"%s supplies %s, which %s already supplies: %w",
					plugin._Label(),
					unpack.QuotePrefix(key),
					first,
					ErrConflict,
				)
			}

			owner[key] = plugin._Label()
			merged[key] = names[key]
		}
	}

	return merged, nil
}
