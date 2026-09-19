// Package json gives a script the json module. Importing it is what enables it.
package json

import (
	starlarkjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/plugin"
)

const NAME = "json"

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-19 21:09: initial creation
func init() {
	plugin.Register(&_Json{})
}

// _Json is the plugin. Empty: what it contributes is a value the library
// already builds, so there is nothing to hold.
type _Json struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-19 21:09: initial creation
func (j *_Json) Name() string {
	return NAME
}

// Values returns the one name this plugin supplies.
//
// The module comes from go.starlark.net rather than being written here: it is
// the library's own encoder, it is already a dependency, and a second JSON
// implementation in the same process is two behaviours a script could tell
// apart.
//
// The import is go.starlark.net/lib/json, not the older go.starlark.net/
// starlarkjson, which the library deprecated. The linter found that; the POC
// this was lifted from still uses the old path, because nothing lints .poc.
//
// Revisions:
//   - 2026-09-19 21:09: initial creation
func (j *_Json) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: starlarkjson.Module,
	}
}
