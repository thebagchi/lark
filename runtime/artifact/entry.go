package artifact

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"
)

// ErrNoMain is returned for a script with no entry point, or one a run cannot
// call with no arguments.
var ErrNoMain = errors.New("artifact: no entry point")

// ENTRY is the one top-level function a final compilation unit must define.
const ENTRY = "main"

// _RequireEntry reports whether tree defines the entry point as a function that
// can be called with no arguments.
//
// The tree is read rather than the globals because the tree carries a position,
// so an entry point that takes arguments is refused at the line that defines
// it, and because this runs before anything has been initialised.
//
// Revisions:
//   - 2026-09-19 18:43: initial creation
func _RequireEntry(tree *syntax.File) error {
	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if !ok {
			continue
		}

		if def.Name.Name != ENTRY {
			continue
		}

		count := _RequiredParams(def)
		if count > 0 {
			return fmt.Errorf(
				"%s at %s must take no required parameters: %w",
				ENTRY,
				def.Name.NamePos,
				ErrNoMain,
			)
		}

		return nil
	}

	return ErrNoMain
}

// _RequiredParams counts the parameters of def that a caller has to supply.
//
// A plain identifier is required; everything else in that list carries its own
// value or absorbs what is passed — a default, *args, **kwargs — so none of
// them stops a zero-argument call from succeeding.
//
// Revisions:
//   - 2026-09-19 18:44: initial creation
func _RequiredParams(def *syntax.DefStmt) int {
	count := 0

	for _, param := range def.Params {
		_, ok := param.(*syntax.Ident)
		if !ok {
			continue
		}

		count++
	}

	return count
}
