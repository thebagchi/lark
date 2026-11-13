package dialect

import (
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect/internal/rewrite"
)

// What a closed suite and a match are, read one way by the dialect, which
// marks a branch's call when it compiles, and by derive, which reads the same
// lines as an If or a Match. Two readings would disagree about some script,
// and a run would then report lines its flow does not have.

// Closed calls rewrite.Closed.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q319)
func Closed(body []syntax.Stmt) []syntax.Stmt {
	return rewrite.Closed(body)
}

// Matched calls rewrite.Matched.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q319)
func Matched(
	body []syntax.Stmt,
	idx int,
) (*syntax.AssignStmt, *syntax.IfStmt, bool) {
	return rewrite.Matched(
		body,
		idx,
	)
}

// NextCase calls rewrite.NextCase.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q319)
func NextCase(chain *syntax.IfStmt) *syntax.IfStmt {
	return rewrite.NextCase(chain)
}

// Tested calls rewrite.Tested.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q319)
func Tested(expr syntax.Expr) (string, bool) {
	return rewrite.Tested(expr)
}
