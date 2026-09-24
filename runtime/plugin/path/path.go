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
package path

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/runtime/plugin"
)

// ErrNotAPath is returned for an argument that is not text.
var ErrNotAPath = errors.New("wants a path as text")

const (
	// NAME is the module, and the names it holds.
	NAME      = "path"
	JOIN      = "join"
	DIR       = "dir"
	BASE      = "base"
	EXT       = "ext"
	STEM      = "stem"
	CLEAN     = "clean"
	SPLIT     = "split"
	PARTS     = "parts"
	ABS       = "isabs"
	SEPARATOR = string(filepath.Separator)
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
type _Path struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (p *_Path) Name() string {
	return NAME
}

// Values returns the path module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (p *_Path) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				JOIN:  starlark.NewBuiltin(NAME+"."+JOIN, _Join),
				SPLIT: starlark.NewBuiltin(NAME+"."+SPLIT, _Split),
				PARTS: starlark.NewBuiltin(NAME+"."+PARTS, _Parts),
				DIR:   _Reads(DIR, filepath.Dir),
				BASE:  _Reads(BASE, filepath.Base),
				EXT:   _Reads(EXT, filepath.Ext),
				STEM:  _Reads(STEM, _Stem),
				CLEAN: _Reads(CLEAN, filepath.Clean),
				ABS:   starlark.NewBuiltin(NAME+"."+ABS, _IsAbs),
			},
		},
	}
}

// _Reads wraps an operation that takes one path and answers with another.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Reads(name string, fn func(string) string) *starlark.Builtin {
	return starlark.NewBuiltin(NAME+"."+name, func(
		thread *starlark.Thread,
		held *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var given string

		err := starlark.UnpackPositionalArgs(held.Name(), args, kwargs, 1, &given)
		if err != nil {
			return nil, err
		}

		return starlark.String(fn(given)), nil
	})
}

// _Join puts parts together with one separator between each.
//
// Takes any number, because a caller building a path builds it from as many
// pieces as it has, and cleans the result, so that join("a/", "/b") is the
// path a reader expects rather than the text they concatenated.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Join(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 0)
	}

	parts := make([]string, 0, len(args))

	for index, given := range args {
		held, ok := starlark.AsString(given)
		if !ok {
			return nil, _NotAPath(fn.Name(), index, given)
		}

		parts = append(parts, held)
	}

	return starlark.String(filepath.Join(parts...)), nil
}

// _Split is a path as its directory and its last element, the pair almost
// every caller wants at once.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Split(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var given string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &given)
	if err != nil {
		return nil, err
	}

	dir, base := filepath.Split(given)

	return starlark.NewList([]starlark.Value{
		starlark.String(strings.TrimSuffix(dir, SEPARATOR)),
		starlark.String(base),
	}), nil
}

// _Parts is every element of a path, in order, with the empties dropped.
//
// A leading separator survives as its own first element, because a path that
// begins at the root and one that begins beside you are different paths and a
// list that lost the difference could not be put back together.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Parts(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var given string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &given)
	if err != nil {
		return nil, err
	}

	held := make([]starlark.Value, 0, strings.Count(given, SEPARATOR)+1)

	if strings.HasPrefix(given, SEPARATOR) {
		held = append(held, starlark.String(SEPARATOR))
	}

	for _, part := range strings.Split(filepath.Clean(given), SEPARATOR) {
		if part == "" || part == "." {
			continue
		}

		held = append(held, starlark.String(part))
	}

	return starlark.NewList(held), nil
}

// _IsAbs reports whether a path begins at the root.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _IsAbs(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var given string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &given)
	if err != nil {
		return nil, err
	}

	return starlark.Bool(filepath.IsAbs(given)), nil
}

// _Stem is the last element with its extension taken off.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Stem(given string) string {
	base := filepath.Base(given)

	return strings.TrimSuffix(base, filepath.Ext(base))
}

// _NotAPath is what join says about an argument that is not text.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _NotAPath(who string, index int, given starlark.Value) error {
	return fmt.Errorf("%s: argument %d is a %s: %w", who, index+1, given.Type(), ErrNotAPath)
}
