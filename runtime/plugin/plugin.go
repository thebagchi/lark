// Package plugin is how a host adds names to the environment a script is given.
//
// A plugin registers itself from its own init, so importing a plugin package is
// the whole of enabling it. What it registers with is a Registry; the default
// one is what an init reaches, and a host that wants a compiler with a
// different set builds its own.
package plugin

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

var ErrConflict = errors.New("two plugins supply one name")

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

// Registry is a set of plugins, in the order they were registered.
//
// A type rather than package state, so a compiler can be given one and two
// compilers in one process can see different plugins. The package-level calls
// below reach DEFAULT, which is where a plugin's init registers and what a
// compiler uses when a host names none.
type Registry struct {
	guard     sync.Mutex
	installed []Plugin
}

// DEFAULT is the registry a plugin's init reaches, and the one a compiler
// uses when a host names none.
var DEFAULT = New()

// New returns a registry holding no plugins.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func New() *Registry {
	return new(Registry)
}

// Register installs plugin.
//
// It returns nothing on purpose. An init cannot handle an error, and CLAUDE.md
// forbids ignoring one, so a conflict is recorded rather than raised: it is
// found when the environment is built, by Environment, which has a caller that
// can act on it.
//
// Revisions:
//   - 2026-09-19 21:04: initial creation, as a package function
//   - 2026-09-21 09:46: a method on the registry it installs into
func (r *Registry) Register(plugin Plugin) {
	r.guard.Lock()
	defer r.guard.Unlock()

	r.installed = append(r.installed, plugin)
}

// Registered returns the plugins installed so far, in registration order.
//
// Revisions:
//   - 2026-09-19 21:05: initial creation
//   - 2026-09-21 09:46: a method on the registry
func (r *Registry) Registered() []Plugin {
	r.guard.Lock()
	defer r.guard.Unlock()

	installed := make([]Plugin, len(r.installed))
	copy(installed, r.installed)

	return installed
}

// Environment merges every registered plugin's names into one environment.
//
// Returns ErrConflict naming both plugins and the name they clash on. A script's
// environment is decided before anything runs, so a clash is refused there
// rather than resolved silently - a shadowed builtin is a defect that surfaces
// much later, as wrong behaviour, somewhere else.
//
// Revisions:
//   - 2026-09-19 21:06: initial creation
//   - 2026-09-21 09:46: a method on the registry
func (r *Registry) Environment() (starlark.StringDict, error) {
	merged := starlark.StringDict{}
	owner := map[string]string{}

	for _, plugin := range r.Registered() {
		names := plugin.Values()

		for _, key := range _Sorted(names) {
			first, taken := owner[key]
			if taken {
				return nil, fmt.Errorf(
					"%s supplies %q, which %s already supplies: %w",
					plugin.Name(),
					key,
					first,
					ErrConflict,
				)
			}

			owner[key] = plugin.Name()
			merged[key] = names[key]
		}
	}

	return merged, nil
}

// Register installs plugin into DEFAULT, and is meant to be called from a
// plugin package's init, so that importing that package is what enables it.
//
// Revisions:
//   - 2026-09-19 21:04: initial creation
//   - 2026-09-21 09:46: reaches DEFAULT
func Register(plugin Plugin) {
	DEFAULT.Register(plugin)
}

// _Sorted returns the keys of names in a fixed order.
//
// Map iteration is randomised, so a plugin supplying two clashing names would
// otherwise report whichever one the runtime happened to reach first, and the
// same conflict would read differently on different runs.
//
// Revisions:
//   - 2026-09-19 21:07: initial creation
func _Sorted(names starlark.StringDict) []string {
	keys := make([]string, 0, len(names))

	for key := range names {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}
