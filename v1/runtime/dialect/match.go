package dialect

import (
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// What a closed suite and a match are, read one way by the dialect, which
// marks a branch's call when it compiles, and by derive, which reads the same
// lines as an If or a Match. Two readings would disagree about some script,
// and a run would then report lines its flow does not have.

// CLOSED is the fewest statements a suite holds when one of them is a closer:
// the closer, and one statement it closes.
const CLOSED = 2

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
func Closed(body []syntax.Stmt) []syntax.Stmt {
	if len(body) < CLOSED {
		return body
	}

	last, ok := body[len(body)-1].(*syntax.BranchStmt)
	if ok && last.Token == syntax.PASS {
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
func Matched(body []syntax.Stmt, idx int) (*syntax.AssignStmt, *syntax.IfStmt, bool) {
	if idx+1 >= len(body) {
		return nil, nil, false
	}

	assign, ok := body[idx].(*syntax.AssignStmt)
	if !ok || assign.Op != syntax.EQ {
		return nil, nil, false
	}

	held, ok := assign.LHS.(*syntax.Ident)
	if !ok || held.Name != spelling.SUBJECT {
		return nil, nil, false
	}

	chain, ok := body[idx+1].(*syntax.IfStmt)
	if !ok {
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
func NextCase(chain *syntax.IfStmt) *syntax.IfStmt {
	if len(chain.False) != 1 {
		return nil
	}

	deeper, ok := chain.False[0].(*syntax.IfStmt)
	if !ok {
		return nil
	}

	if _, tested := Tested(deeper.Cond); !tested {
		return nil
	}

	return deeper
}

// Tested is the string an arm of a match's chain compares the subject to, and
// whether the arm compares it to one.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation, as derive's _Tested
//   - 2026-10-02 01:13: lifted into the dialect and exported, so the dialect
//     reads a match's arms as derive does
func Tested(expr syntax.Expr) (string, bool) {
	compared, ok := expr.(*syntax.BinaryExpr)
	if !ok || compared.Op != syntax.EQL {
		return "", false
	}

	held, ok := compared.X.(*syntax.Ident)
	if !ok || held.Name != spelling.SUBJECT {
		return "", false
	}

	literal, ok := compared.Y.(*syntax.Literal)
	if !ok {
		return "", false
	}

	text, ok := literal.Value.(string)

	return text, ok
}
