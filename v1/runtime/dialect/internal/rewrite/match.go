package rewrite

import (
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect/internal/spec"
)

// Closed is a suite without its closing pass: the pass that ends a suite
// holding at least one other statement.
//
// A suite that is only pass keeps it, because that pass is the whole of the
// suite rather than a closer, and a flow stores it as text.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation, from _Trimmed, which dropped the
//     def's own pass and nothing else
//   - 2026-10-02 01:13: lifted out of derive and exported, so the dialect
//     sets a closer aside as derive does
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func Closed(body []syntax.Stmt) []syntax.Stmt {
	if len(body) < spec.CLOSED {
		return body
	}

	last, ok := body[len(body)-1].(*syntax.BranchStmt)
	if ok && last != nil && last.Token == syntax.PASS {
		return body[:len(body)-1]
	}

	return body
}

// Matched is the assignment and the chain that make the statement at idx and
// the one after it a match, and whether they do.
//
// An assignment to the subject, immediately followed by an if comparing the
// subject to a string. An if on any other name, or an assignment to the
// subject that no such if follows, is not a match.
//
// Revisions:
//   - 2026-10-02 01:13: initial creation, from derive's _Matched, so the
//     dialect and derive read a match one way
//   - 2026-11-04 19:30: a negative index or a nil node is no match (C87)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func Matched(
	body []syntax.Stmt,
	idx int,
) (*syntax.AssignStmt, *syntax.IfStmt, bool) {
	if idx < 0 || idx+1 >= len(body) {
		return nil, nil, false
	}

	assign, ok := body[idx].(*syntax.AssignStmt)
	if !ok || assign == nil || assign.Op != syntax.EQ {
		return nil, nil, false
	}

	held, ok := assign.LHS.(*syntax.Ident)
	if !ok || held == nil || held.Name != common.SUBJECT {
		return nil, nil, false
	}

	chain, ok := body[idx+1].(*syntax.IfStmt)
	if !ok || chain == nil {
		return nil, nil, false
	}

	if _, tested := Tested(chain.Cond); !tested {
		return nil, nil, false
	}

	return assign, chain, true
}

// NextCase is the if holding a match chain's next case, or nil: chain's else,
// when the else is one if comparing the subject. Any other else is the match's
// default.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation, from derive's _Cases, so the dialect
//     and derive walk a chain's cases one way
//   - 2026-11-04 19:30: a nil chain or a nil else has no next case (C87)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func NextCase(chain *syntax.IfStmt) *syntax.IfStmt {
	if chain == nil || len(chain.False) != 1 {
		return nil
	}

	deeper, ok := chain.False[0].(*syntax.IfStmt)
	if !ok || deeper == nil {
		return nil
	}

	if _, tested := Tested(deeper.Cond); !tested {
		return nil
	}

	return deeper
}

// Tested is the string an arm of a match's chain compares the subject to, and
// whether the arm compares it to one. A bytes literal is not one: go.starlark.net
// keeps its value as a Go string too, and b"a" is not equal to "a" (B290).
//
// Revisions:
//   - 2026-09-21 01:32: initial creation, as derive's _Tested
//   - 2026-10-02 01:13: lifted into the dialect and exported, so the dialect
//     reads a match's arms as derive does
//   - 2026-11-04 19:30: a nil node is no arm (C87)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
//   - 2026-11-12 23:35: a bytes literal is no string an arm compares to (C130, B290, Q403)
func Tested(expr syntax.Expr) (string, bool) {
	compared, ok := expr.(*syntax.BinaryExpr)
	if !ok || compared == nil || compared.Op != syntax.EQL {
		return "", false
	}

	held, ok := compared.X.(*syntax.Ident)
	if !ok || held == nil || held.Name != common.SUBJECT {
		return "", false
	}

	literal, ok := compared.Y.(*syntax.Literal)
	if !ok || literal == nil {
		return "", false
	}

	text, ok := literal.Value.(string)
	if !ok || literal.Token == syntax.BYTES {
		return "", false
	}

	return text, true
}
