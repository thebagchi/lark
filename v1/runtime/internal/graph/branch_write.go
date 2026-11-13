package graph

import (
	"fmt"
	"strconv"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// One direction of one subject lives here: writing an if and a match back as a
// script. A branch is one statement; an elif is an If held as the else of
// another; a match is an assignment to _match and the chain that compares it.
// The other direction is in branch_read.go.

// If is an if. An else whose statement is itself an if is an elif, which
// _Elif marks, and which is how a chain is stored.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the if node
//   - 2026-10-02 00:04: lifted into graph, taking the generated If
//   - 2026-10-24 18:29: an elif is _Elif, so this takes no flag
//   - 2026-11-02 10:25: moved to branch_write.go
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses If, Program, were _If, _Program (C120a, Q314)
func _If(
	iff *workflowpb.If,
	prog *Program,
) (*syntax.IfStmt, error) {
	if iff == nil || iff.GetThen() == nil {
		return nil, fmt.Errorf(
			"if: function has no body: %w",
			ErrForm,
		)
	}

	cond, err := _Condition(iff.GetCondition())
	if err != nil {
		return nil, err
	}

	then, err := _Statement(
		iff.GetThen(),
		prog,
	)
	if err != nil {
		return nil, err
	}

	node := &syntax.IfStmt{
		Cond: cond,
		True: then,
	}

	return _Else(
		node,
		iff.GetElse(),
		prog,
	)
}

// _Elif is an if that continues another if.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:25: moved to branch_write.go
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Elif(
	iff *workflowpb.If,
	prog *Program,
) (*syntax.IfStmt, error) {
	node, err := _If(
		iff,
		prog,
	)
	if err != nil {
		return nil, err
	}

	prog.elif[node] = struct{}{}

	return node, nil
}

// _Else sets the other branch. Absent means the script wrote none. An if
// there is an elif.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: attaches the branch to the if
//   - 2026-10-02 00:04: lifted into graph, taking the generated Statement
//   - 2026-11-02 10:25: moved to branch_write.go
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Else(
	node *syntax.IfStmt,
	other *workflowpb.Statement,
	prog *Program,
) (*syntax.IfStmt, error) {
	if other == nil {
		return node, nil
	}

	if nested := other.GetIf(); nested != nil {
		inner, err := _Elif(
			nested,
			prog,
		)
		if err != nil {
			return nil, err
		}

		node.False = []syntax.Stmt{inner}

		return node, nil
	}

	body, err := _Statement(
		other,
		prog,
	)
	if err != nil {
		return nil, err
	}

	node.False = body

	return node, nil
}

// Condition is a bool, a call, or a name.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Condition
//   - 2026-11-02 10:25: moved to branch_write.go
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-10 01:00: comment now names Condition, were _Condition (C120a, Q314)
func _Condition(cond *workflowpb.Condition) (syntax.Expr, error) {
	if cond == nil {
		return nil, fmt.Errorf(
			"condition: %w",
			ErrForm,
		)
	}

	switch kind := cond.GetKind().(type) {
	case *workflowpb.Condition_Value:
		spelled := FALSE
		if kind.Value {
			spelled = TRUE
		}

		return _Name(spelled), nil

	case *workflowpb.Condition_Name:
		if kind.Name == "" {
			return nil, fmt.Errorf(
				"condition: %w",
				ErrForm,
			)
		}

		return _Name(kind.Name), nil

	case *workflowpb.Condition_Call:
		if kind.Call.GetResult() != "" {
			return nil, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(kind.Call.GetFunction()),
				ErrForm,
			)
		}

		return _CallExpr(kind.Call)

	default:
		return nil, fmt.Errorf(
			"condition: %w",
			ErrForm,
		)
	}
}

// _Match is an assignment to _match and the if/elif/else that compares it.
//
// The binding's name is not stored. The first case is if, and each case
// after it is elif. No default means no else.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the assignment and the comparisons
//   - 2026-10-02 00:04: lifted into graph, taking the generated Match
//   - 2026-11-02 10:25: moved to branch_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Match(
	match *workflowpb.Match,
	prog *Program,
) ([]syntax.Stmt, error) {
	if match == nil || len(match.GetCases()) == 0 {
		return nil, fmt.Errorf(
			"match: %w",
			ErrForm,
		)
	}

	expr, err := _Expression(match.GetExpression())
	if err != nil {
		return nil, err
	}

	top, err := _Chain(
		match,
		prog,
	)
	if err != nil {
		return nil, err
	}

	assign := &syntax.AssignStmt{
		Op:  syntax.EQ,
		LHS: _Name(common.SUBJECT),
		RHS: expr,
	}

	return []syntax.Stmt{assign, top}, nil
}

// _Chain is the comparisons, the first an if and each one after it an elif.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-02 00:04: lifted into graph, taking the generated Match
//   - 2026-11-02 10:25: moved to branch_write.go
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Chain(
	match *workflowpb.Match,
	prog *Program,
) (*syntax.IfStmt, error) {
	var (
		top   *syntax.IfStmt
		chain *syntax.IfStmt
	)

	first := true

	for _, item := range match.GetCases() {
		arm, err := _Arm(
			item,
			prog,
		)
		if err != nil {
			return nil, err
		}

		if first {
			top = arm
			first = false
		} else {
			prog.elif[arm] = struct{}{}
			chain.False = []syntax.Stmt{arm}
		}

		chain = arm
	}

	if match.GetDefault() == nil {
		return top, nil
	}

	body, err := _Statement(
		match.GetDefault(),
		prog,
	)
	if err != nil {
		return nil, err
	}

	chain.False = body

	return top, nil
}

// _Arm is one comparison.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the if node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Case
//   - 2026-11-02 10:25: moved to branch_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Arm(
	item *workflowpb.Case,
	prog *Program,
) (*syntax.IfStmt, error) {
	if item == nil || item.GetStatement() == nil {
		return nil, fmt.Errorf(
			"match: function has no body: %w",
			ErrForm,
		)
	}

	body, err := _Statement(
		item.GetStatement(),
		prog,
	)
	if err != nil {
		return nil, err
	}

	cond := &syntax.BinaryExpr{
		X:  _Name(common.SUBJECT),
		Op: syntax.EQL,
		Y:  _Text(strconv.Quote(item.GetValue())),
	}

	return &syntax.IfStmt{
		Cond: cond,
		True: body,
	}, nil
}

// Expression is a string, a call, or a name.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Expression
//   - 2026-11-02 10:25: moved to branch_write.go
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-10 01:00: comment now names Expression, were _Expression (C120a, Q314)
func _Expression(expr *workflowpb.Expression) (syntax.Expr, error) {
	if expr == nil {
		return nil, fmt.Errorf(
			"expression: %w",
			ErrForm,
		)
	}

	switch kind := expr.GetKind().(type) {
	case *workflowpb.Expression_Value:
		return _Text(strconv.Quote(kind.Value)), nil

	case *workflowpb.Expression_Name:
		if kind.Name == "" {
			return nil, fmt.Errorf(
				"expression: %w",
				ErrForm,
			)
		}

		return _Name(kind.Name), nil

	case *workflowpb.Expression_Call:
		if kind.Call.GetResult() != "" {
			return nil, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(kind.Call.GetFunction()),
				ErrForm,
			)
		}

		return _CallExpr(kind.Call)

	default:
		return nil, fmt.Errorf(
			"expression: %w",
			ErrForm,
		)
	}
}
