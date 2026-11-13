package artifact

import (
	"fmt"
	"slices"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Owners is the name of the plugin that supplies each name the plugins give.
//
// Read here, from the keys of Values, because the plugin package offers no
// public way to ask: a name added to its API would be visible to every host.
// Only the keys are read, so a plugin that builds its values on each call
// gives no different answer. A clash is refused earlier, by the environment.
//
// Revisions:
//   - 2026-11-05 17:24: initial creation (C79, Q181)
//   - 2026-11-05 18:40: reads Values and Name through _Hosted, and returns its error
//     (B123, C96 fix round 2)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
func _Owners(plugins []common.Plugin) (map[string]string, error) {
	var owners = map[string]string{}

	for _, each := range plugins {
		if each == nil {
			continue
		}

		var (
			names starlark.StringDict
			owner string
		)

		err := _Hosted(
			each,
			"Values",
			func() {
				names = each.Values()
			},
		)
		if err != nil {
			return nil, err
		}

		err = _Hosted(
			each,
			"Name",
			func() {
				owner = each.Name()
			},
		)
		if err != nil {
			return nil, err
		}

		for name := range names {
			owners[name] = owner
		}
	}

	return owners, nil
}

// _Required is the names of the plugins that supply a name tree uses, for the
// plugins given: _Owners, then _Needed. It returns the error of a plugin that
// panics when asked.
//
// Revisions:
//   - 2026-11-05 18:55: initial creation, to keep Compile short (B123, C96 fix round 2)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
func _Required(
	tree *syntax.File,
	plugins []common.Plugin,
) ([]string, error) {
	owners, err := _Owners(plugins)
	if err != nil {
		return nil, err
	}

	return _Needed(
		tree,
		owners,
	), nil
}

// _Needed is the names of the plugins that supply a name tree uses, sorted and
// without repeats.
//
// tree must be resolved, which a tree is once it is compiled. A name counts
// when the resolver bound it as predeclared, so a local or a global that
// shadows a plugin's name does not. owners says which plugin supplies each
// predeclared name; a name with no owner, such as the dialect's CALL, is left
// out. A plugin with no name is left out too, since a bundle could not say
// which it needs.
//
// Revisions:
//   - 2026-11-05 16:54: initial creation (C79, B111)
func _Needed(
	tree *syntax.File,
	owners map[string]string,
) []string {
	var used = map[string]bool{}

	syntax.Walk(
		tree,
		func(node syntax.Node) bool {
			name, ok := node.(*syntax.Ident)
			if !ok {
				return true
			}

			bound, ok := name.Binding.(*resolve.Binding)
			if !ok || bound.Scope != resolve.Predeclared {
				return true
			}

			owner := owners[name.Name]
			if owner != "" {
				used[owner] = true
			}

			return true
		},
	)

	needed := make(
		[]string,
		0,
		len(used),
	)

	for owner := range used {
		needed = append(
			needed,
			owner,
		)
	}

	slices.Sort(needed)

	return needed
}

// _Supplied refuses a bundle that names a plugin the load was not given. The
// plugin is quoted and cut, since the name is text the file holds.
//
// Compared by name, which is what a conflict reports a plugin by. A bundle that
// names no plugin, as one written before the field existed, passes.
//
// Revisions:
//   - 2026-11-05 16:54: initial creation (C79, B111)
//   - 2026-11-05 18:40: reads Name through _Hosted (B123, C96 fix round 2)
//   - 2026-11-07 18:16: common.Plugin and common.Checking in place of the plugin names (C91w1)
//   - 2026-11-10 17:15: uses ErrPlugin, formerly ErrPluginMissing (C121, Q348)
func _Supplied(
	needed []string,
	given []common.Plugin,
) error {
	var held = make(
		map[string]bool,
		len(given),
	)

	for _, each := range given {
		if each == nil {
			continue
		}

		var label string

		err := _Hosted(
			each,
			"Name",
			func() {
				label = each.Name()
			},
		)
		if err != nil {
			return err
		}

		held[label] = true
	}

	for _, name := range needed {
		if !held[name] {
			return fmt.Errorf(
				"plugin %s: %w",
				unpack.QuotePrefix(name),
				ErrPlugin,
			)
		}
	}

	return nil
}
