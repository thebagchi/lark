// Package path gives a script the operations on a path as text. Importing it
// is what enables it.
//
// Text only. Nothing here touches a disk, so every answer is the same whether
// the path exists or not - which is what separates this from file, where every
// answer depends on what is actually there.
//
// The host's own separator, through path/filepath, rather than a slash
// everywhere. These paths are handed to file, which hands them to the
// operating system, so a module that spelled them its own way would be
// building something the host then has to translate - and the translation is
// exactly where a path stops meaning what it said. The cost is real and worth
// naming: the same script reads a different separator on Windows, so a
// workflow that compares a built path against a literal one written with
// slashes will not match there. Build paths with join rather than with text,
// and that never arises.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package path

import (
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/path/internal/segment"
	"github.com/thebagchi/lark/v1/runtime/plugin/path/internal/spec"
)

var (
	// ErrPath is segment.ErrPath.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	//   - 2026-11-10 17:15: renamed from ErrNotAPath (C121, Q349)
	ErrPath = segment.ErrPath
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func init() {
	plugin.Register(new(_Path))
}

// _Path is the plugin. Empty: every operation works on the text it is given.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Path struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (p *_Path) Name() string {
	return spec.NAME
}

// Values returns the path module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 11:32: uses segment.Join, segment.Split, segment.Parts and segment.IsAbs,
//     which moved to internal/segment (C120b, Q315)
func (p *_Path) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.JOIN: starlark.NewBuiltin(
					spec.NAME+"."+spec.JOIN,
					segment.Join,
				),
				spec.SPLIT: starlark.NewBuiltin(
					spec.NAME+"."+spec.SPLIT,
					segment.Split,
				),
				spec.PARTS: starlark.NewBuiltin(
					spec.NAME+"."+spec.PARTS,
					segment.Parts,
				),
				spec.DIR: _Reads(
					spec.DIR,
					filepath.Dir,
				),
				spec.BASE: _Reads(
					spec.BASE,
					filepath.Base,
				),
				spec.EXT: _Reads(
					spec.EXT,
					filepath.Ext,
				),
				spec.STEM: _Reads(
					spec.STEM,
					_Stem,
				),
				spec.CLEAN: _Reads(
					spec.CLEAN,
					filepath.Clean,
				),
				spec.ABS: starlark.NewBuiltin(
					spec.NAME+"."+spec.ABS,
					segment.IsAbs,
				),
			},
		},
	}
}

// _Reads wraps an operation that takes one path and answers with another.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func _Reads(
	name string,
	fn func(string) string,
) *starlark.Builtin {
	return starlark.NewBuiltin(
		spec.NAME+"."+name,
		func(
			thread *starlark.Thread,
			held *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			var given string

			err := starlark.UnpackPositionalArgs(
				shared.Called(held),
				args,
				kwargs,
				1,
				&given,
			)
			if err != nil {
				return nil, err
			}

			return starlark.String(fn(given)), nil
		},
	)
}

// _Stem is the last element with its extension taken off.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Stem(given string) string {
	base := filepath.Base(given)

	return strings.TrimSuffix(
		base,
		filepath.Ext(base),
	)
}
