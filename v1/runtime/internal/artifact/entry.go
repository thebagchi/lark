package artifact

import (
	"fmt"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// _RequireEntry reports whether tree defines the entry point as a function that
// can be called with no arguments.
//
// The tree is read rather than the globals because the tree carries a position,
// so an entry point that takes arguments is refused at the line that defines
// it, and because this runs before anything has been initialised.
//
// Revisions:
//   - 2026-09-19 18:43: initial creation
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 16:05: cuts the file name in the position to common.QUOTE_LIMIT bytes (C117,
//     B239, Q4)
//   - 2026-11-10 17:15: uses ErrMain, formerly ErrNoMain (C121, Q348)
func _RequireEntry(tree *syntax.File) error {
	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if !ok {
			continue
		}

		if def.Name.Name != common.ENTRY {
			continue
		}

		count := _RequiredParams(def)
		if count > 0 {
			return fmt.Errorf(
				"%s at %s must take no required parameters: %w",
				common.ENTRY,
				_Where(def.Name.NamePos),
				spelling.ErrMain,
			)
		}

		return nil
	}

	return spelling.ErrMain
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

// _Where is pos as syntax.Position.String writes it, with the file name cut to
// common.QUOTE_LIMIT bytes (Q4). The file is the entry or a module, a name a
// host or a bundle chose.
//
// Revisions:
//   - 2026-11-10 16:05: initial creation (C117, B239, Q4)
func _Where(pos syntax.Position) string {
	file := unpack.ShortText(pos.Filename())

	switch {
	case pos.Line <= 0:
		return file

	case pos.Col <= 0:
		return fmt.Sprintf(
			"%s:%d",
			file,
			pos.Line,
		)
	}

	return fmt.Sprintf(
		"%s:%d:%d",
		file,
		pos.Line,
		pos.Col,
	)
}
