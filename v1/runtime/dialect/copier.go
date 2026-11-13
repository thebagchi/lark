package dialect

import (
	"fmt"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect/internal/rewrite"
	"github.com/thebagchi/lark/v1/runtime/guard"
)

var (
	// ErrUncopied is rewrite.ErrUncopied.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrUncopied = rewrite.ErrUncopied
)

// Copied is a copy of file that shares no node with it, so resolving one tree
// writes nothing into the other.
//
// file must not yet be resolved: a node is copied whole, and the copy of a
// resolved node would carry the resolver's binding.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-02 22:58: exported, so a merge copies the files it reads (C44a)
//   - 2026-11-04 19:30: a nil file, or a node that panics the copy (a typed
//     nil, a node with a part missing), is ErrUncopied (C87)
func Copied(file *syntax.File) (*syntax.File, error) {
	var (
		fresh *syntax.File
		err   error
	)

	raised := guard.Contained(func() {
		fresh, err = _Copy(file)
	})
	if raised != nil {
		return nil, fmt.Errorf(
			"%w: %w",
			ErrUncopied,
			raised,
		)
	}

	return fresh, err
}

// _Copy copies file. A panic from a node with a part missing leaves here and
// is caught by Copied.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation, the body Copied had (C87)
//   - 2026-11-08 12:59: reads the refusal under the guard (C108, B198, Q207)
//   - 2026-11-10 11:32: uses rewrite.Copier, which moved to internal/rewrite (C120b, Q315)
func _Copy(file *syntax.File) (*syntax.File, error) {
	var (
		copier = new(rewrite.Copier)
		fresh  = *file
	)

	fresh.Stmts = copier.Stmts(file.Stmts)

	err := copier.Failed()
	if err != nil {
		return nil, err
	}

	return &fresh, nil
}
