package jsonpath

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/patch"
	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/spec"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-20 01:12: initial creation
func init() {
	plugin.Register(&_Plugin{})
}

// _Plugin is the plugin. Empty: every call works on the document it is given,
// so there is nothing to hold.
//
// Revisions:
//   - 2026-09-20 00:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Plugin struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 01:12: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
func (p *_Plugin) Name() string {
	return spec.NAME
}

// Values returns the five names this plugin supplies.
//
// They are flat rather than members of a module, because that is how §10.7 of
// the brief spells them and how the scripts written against it call them.
//
// Revisions:
//   - 2026-09-20 01:13: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 11:32: uses patch.Apply, patch.FindKey, patch.ExtractJSON, patch.MatchJSON and
//     patch.LenJSON, which moved to internal/patch (C120b, Q315)
func (p *_Plugin) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.PATCH: starlark.NewBuiltin(
			spec.PATCH,
			patch.Apply,
		),
		spec.FIND: starlark.NewBuiltin(
			spec.FIND,
			patch.FindKey,
		),
		spec.EXTRACT: starlark.NewBuiltin(
			spec.EXTRACT,
			patch.ExtractJSON,
		),
		spec.MATCH: starlark.NewBuiltin(
			spec.MATCH,
			patch.MatchJSON,
		),
		spec.LENGTH: starlark.NewBuiltin(
			spec.LENGTH,
			patch.LenJSON,
		),
	}
}
