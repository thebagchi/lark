// Package segment holds the path builtins of the path plugin that cut a path
// into segments or join segments into a path: is_abs, join, parts and split. The
// plugin type and its registration stay in package path.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package segment

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"

	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/path/internal/spec"
)

// ErrPath is returned for an argument that is not text.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/path (C120b, Q315)
//   - 2026-11-10 17:15: renamed from ErrNotAPath (C121, Q349)
var (
	ErrPath = errors.New("wants a path as text")
)

// Join puts parts together with one separator between each.
//
// Takes any number, because a caller building a path builds it from as many
// pieces as it has, and cleans the result, so that join("a/", "/b") is the
// path a reader expects rather than the text they concatenated.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/path and renamed _Join to Join, for the
//     tests in tests/ (C120b, Q315, Q314)
func Join(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, starlark.UnpackPositionalArgs(
			shared.Called(fn),
			args,
			kwargs,
			0,
		)
	}

	parts := make(
		[]string,
		0,
		len(args),
	)

	for index, given := range args {
		held, ok := starlark.AsString(given)
		if !ok {
			return nil, _NotAPath(
				shared.Called(fn),
				index,
				given,
			)
		}

		parts = append(
			parts,
			held,
		)
	}

	return starlark.String(filepath.Join(parts...)), nil
}

// Split is a path as its directory and its last element, the pair almost
// every caller wants at once.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/path and renamed _Split to Split, for the
//     tests in tests/ (C120b, Q315, Q314)
func Split(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var given string

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&given,
	)
	if err != nil {
		return nil, err
	}

	dir, base := filepath.Split(given)

	return starlark.NewList([]starlark.Value{
		starlark.String(strings.TrimSuffix(
			dir,
			spec.SEPARATOR,
		)),
		starlark.String(base),
	}), nil
}

// Parts is every element of a path, in order, with the empties dropped.
//
// A leading separator survives as its own first element, because a path that
// begins at the root and one that begins beside you are different paths and a
// list that lost the difference could not be put back together.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/path and renamed _Parts to Parts, for the
//     tests in tests/ (C120b, Q315, Q314)
func Parts(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var given string

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&given,
	)
	if err != nil {
		return nil, err
	}

	return starlark.NewList(_SplitPath(given)), nil
}

// _SplitPath is the parts of a path, with a leading separator kept.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/path (C120b, Q315)
func _SplitPath(given string) []starlark.Value {
	held := make(
		[]starlark.Value,
		0,
		strings.Count(
			given,
			spec.SEPARATOR,
		)+1,
	)

	if strings.HasPrefix(
		given,
		spec.SEPARATOR,
	) {
		held = append(
			held,
			starlark.String(spec.SEPARATOR),
		)
	}

	for _, part := range strings.Split(
		filepath.Clean(given),
		spec.SEPARATOR,
	) {
		if part == "" || part == "." {
			continue
		}

		held = append(
			held,
			starlark.String(part),
		)
	}

	return held
}

// IsAbs reports whether a path begins at the root.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/path and renamed _IsAbs to IsAbs, for the
//     tests in tests/ (C120b, Q315, Q314)
func IsAbs(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var given string

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&given,
	)
	if err != nil {
		return nil, err
	}

	return starlark.Bool(filepath.IsAbs(given)), nil
}

// _NotAPath is what join says about an argument that is not text.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/path (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrPath, formerly ErrNotAPath (C121, Q349)
func _NotAPath(
	who string,
	index int,
	given starlark.Value,
) error {
	return fmt.Errorf(
		"%s: argument %d is a %s: %w",
		who,
		index+1,
		given.Type(),
		ErrPath,
	)
}
