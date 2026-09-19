// Package plugin is how a host adds names to the environment a script is given.
//
// A plugin registers itself from its own init, so importing a plugin package is
// the whole of enabling it.
package plugin

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"go.starlark.net/starlark"
)

var ErrConflict = errors.New("plugin: two plugins supply one name")

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

var (
	GUARD    sync.Mutex
	REGISTRY []Plugin
)

// Register installs plugin, and is meant to be called from a plugin package's
// init, so that importing that package is what enables it.
//
// It returns nothing on purpose. An init cannot handle an error, and `CLAUDE.md`
// forbids ignoring one, so a conflict is recorded rather than raised: it is
// found when the environment is built, by Environment, which has a caller that
// can act on it.
//
// Revisions:
//   - 2026-09-19 21:04: initial creation
func Register(plugin Plugin) {
	GUARD.Lock()
	defer GUARD.Unlock()

	REGISTRY = append(REGISTRY, plugin)
}

// Registered returns the plugins installed so far, in registration order.
//
// Revisions:
//   - 2026-09-19 21:05: initial creation
func Registered() []Plugin {
	GUARD.Lock()
	defer GUARD.Unlock()

	installed := make([]Plugin, len(REGISTRY))
	copy(installed, REGISTRY)

	return installed
}

// Reset empties the registry.
//
// It exists because the registry is package-level state, and a test that
// registers anything changes what every later test in the process sees. That is
// the cost of registration being global rather than per-compiler, and this
// function is where the cost is paid rather than hidden.
//
// Revisions:
//   - 2026-09-19 21:05: initial creation
func Reset() {
	GUARD.Lock()
	defer GUARD.Unlock()

	REGISTRY = nil
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
func Environment() (starlark.StringDict, error) {
	merged := starlark.StringDict{}
	owner := map[string]string{}

	for _, plugin := range Registered() {
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
