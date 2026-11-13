package tests

import (
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// _Compile parses src, makes the two trees Compile takes, and compiles them:
// the tree as it was written, resolved, beside the program.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from what Compile did itself
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Compile(
	path string,
	src []byte,
	predeclared func(string) bool,
) (*syntax.File, *starlark.Program, error) {
	written, err := common.OPTIONS.Parse(
		path,
		src,
		0,
	)
	if err != nil {
		return nil, nil, err
	}

	compiled, err := dialect.Copied(written)
	if err != nil {
		return nil, nil, err
	}

	err = resolve.File(
		written,
		predeclared,
		starlark.Universe.Has,
	)
	if err != nil {
		return nil, nil, err
	}

	program, err := dialect.Compile(
		written,
		compiled,
		predeclared,
	)
	if err != nil {
		return nil, nil, err
	}

	return written, program, nil
}
