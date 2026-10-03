// Package plugin is how names reach the environment a script is given.
//
// The runtime's own plugins register themselves from their init, into DEFAULT,
// so importing one is the whole of enabling it, and every compile starts from
// what DEFAULT holds. A host never registers one: it hands a compile its own
// plugins with WithPlugins, which adds them to that compile alone.
package plugin

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

var ERR_CONFLICT = errors.New("two plugins supply one name")

// Checking is a plugin that can refuse a use of its names by reading the
// source, before anything runs.
//
// Optional: a compiler asks every plugin that implements it and leaves the
// rest alone. That is what keeps the knowledge in the right place - a plugin
// knows what its own names mean, and a compiler that had to know would be a
// compiler coupled to every plugin anybody writes.
//
// A tree, not a string, because the question is about what was written rather
// than how it was spelled. Returning an error refuses the compile.
type Checking interface {
	Check(tree *syntax.File) error
}

// Plugin is a set of names a script is given.
//
// The interface is declared here rather than borrowed so that what this package
// depends on is the two things it asks: what a plugin is called, and what it
// contributes. The name is what makes a conflict reportable - without it a
// clash can only name the key, not who supplied it.
type Plugin interface {
	Name() string
	Values() starlark.StringDict
}

// DEFAULT is every plugin of the runtime's own, in the order their inits
// registered them, and what every compile starts from.
//
// A list rather than a type holding one: it is filled once, by inits, which run
// one at a time before anything reads it, so there is nothing to guard. A
// caller adding to it copies it first - slices.Concat - since appending to a
// shared list writes into whatever spare room the next caller sees too.
var DEFAULT []Plugin

// Register adds plugin to DEFAULT, and is meant to be called from the init of
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
func Register(plugin Plugin) {
	DEFAULT = append(DEFAULT, plugin)
}

// Environment merges the names every plugin supplies into one environment, in
// the order the plugins are given.
//
// Returns ERR_CONFLICT naming both plugins and the name they clash on. A script's
// environment is decided before anything runs, so a clash is refused there
// rather than resolved silently - a shadowed builtin is a defect that surfaces
// much later, as wrong behaviour, somewhere else.
//
// Revisions:
//   - 2026-09-19 21:06: initial creation
//   - 2026-09-21 09:46: a method on the registry
//   - 2026-10-03 08:27: a function of the plugins given, there being no registry
func Environment(plugins []Plugin) (starlark.StringDict, error) {
	merged := starlark.StringDict{}
	owner := map[string]string{}

	for _, plugin := range plugins {
		names := plugin.Values()

		// In a fixed order, Keys being sorted: map iteration is randomised, and
		// the same clash would otherwise name a different name on each run.
		for _, key := range names.Keys() {
			first, taken := owner[key]
			if taken {
				return nil, fmt.Errorf(
					"%s supplies %q, which %s already supplies: %w",
					plugin.Name(),
					key,
					first,
					ERR_CONFLICT,
				)
			}

			owner[key] = plugin.Name()
			merged[key] = names[key]
		}
	}

	return merged, nil
}
