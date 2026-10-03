package graph

import (
	"fmt"
	"strconv"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// Both directions of one subject live here: reading an if and a match out of a
// script, and writing them back. A branch is one statement; an elif is an If
// held as the else of another; a match is an assignment to _match and the chain
// that compares it.

// _If is the statement an if is, and whether it models.
//
// Each branch is one statement once its closer is set aside. An elif is the
// else holding one if, which models as an If nested in the else. A branch of
// two statements, or a condition that is a comparison, is not an If.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation, as _Branch
//   - 2026-09-30 00:41: reads on a lane, so a branch's calls may pass a
//     parameter
//   - 2026-10-02 00:04: each branch is a Statement, read in a scope of its
//     own, so an elif nests
func (r *_Reading) _If(stmt *syntax.IfStmt, scope *_Scope) (*workflowpb.Statement, bool) {
	condition, ok := r._Condition(stmt.Cond, scope)
	if !ok {
		return nil, false
	}

	then, ok := r._Branch(stmt.True, scope._Inner())
	if !ok {
		return nil, false
	}

	branch := &workflowpb.If{Condition: condition, Then: then}

	if len(stmt.False) > 0 {
		branch.Else, ok = r._Branch(stmt.False, scope._Inner())
		if !ok {
			return nil, false
		}
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_If{If: branch}}, true
}

// _Condition is the Condition an expression states, or false.
//
// True or False, a call of a listed function, or a name the scope sees. A
// comparison, a negation or anything else states no condition this schema can
// carry.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-30 00:41: reads on a lane, so a condition may pass a parameter
//   - 2026-10-02 00:04: a name is a condition, and the call is read in a scope
func (r *_Reading) _Condition(expr syntax.Expr, scope *_Scope) (*workflowpb.Condition, bool) {
	switch actual := expr.(type) {
	case *syntax.Ident:
		value, word := _Word(actual.Name)
		if word {
			flag, ok := value.GetKind().(*structpb.Value_BoolValue)
			if !ok {
				return nil, false
			}

			return &workflowpb.Condition{
				Kind: &workflowpb.Condition_Value{Value: flag.BoolValue},
			}, true
		}

		if !scope._Sees(actual.Name) {
			return nil, false
		}

		named := &workflowpb.Condition_Name{Name: actual.Name}

		return &workflowpb.Condition{Kind: named}, true

	case *syntax.CallExpr:
		held, ok := r._Call(actual, scope)
		if !ok {
			return nil, false
		}

		return &workflowpb.Condition{Kind: &workflowpb.Condition_Call{Call: held}}, true
	}

	return nil, false
}

// _Branch is the one statement a branch, a case or a default is, and whether
// it models.
//
// An empty branch is one the script did not write, which the caller handles by
// leaving the field empty; this is never handed one.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation, as _Only
//   - 2026-09-30 00:41: reads on a lane, so the call may pass a parameter
//   - 2026-10-02 00:04: any one Statement, once the closer is set aside, rather
//     than a call alone
func (r *_Reading) _Branch(stmts []syntax.Stmt, scope *_Scope) (*workflowpb.Statement, bool) {
	body := dialect.Closed(stmts)
	if len(body) != 1 {
		return nil, false
	}

	return r._Statement(body[0], scope)
}

// _Matched is the Match the statements at this position state together, or
// nil.
//
// Only the shape the walk reads as a match: an assignment to _match, from a
// call, a string or a name, immediately followed by a chain comparing _match
// to string literals. A chain on any other name is an If whose else is an If.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-30 00:41: reads on a lane, so a matched call may pass a
//     parameter
//   - 2026-10-02 00:04: reads the assignment and its chain itself, in a
//     scope, and takes a string or a name as well as a call
//   - 2026-10-02 01:14: finds the assignment and its chain through
//     dialect.Matched, which the dialect reads a match with too
func (r *_Reading) _Matched(body []syntax.Stmt, idx int, scope *_Scope) *workflowpb.Statement {
	assign, chain, ok := dialect.Matched(body, idx)
	if !ok {
		return nil
	}

	expression, ok := r._Expression(assign.RHS, scope)
	if !ok {
		return nil
	}

	match := &workflowpb.Match{Expression: expression}

	if !r._Cases(match, chain, scope) {
		return nil
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Match{Match: match}}
}

// _Expression is the Expression a match evaluates, or false: a string, a call
// of a listed function, or a name the scope sees.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Expression(expr syntax.Expr, scope *_Scope) (*workflowpb.Expression, bool) {
	switch actual := expr.(type) {
	case *syntax.Literal:
		text, ok := actual.Value.(string)
		if !ok {
			return nil, false
		}

		value := &workflowpb.Expression_Value{Value: text}

		return &workflowpb.Expression{Kind: value}, true

	case *syntax.Ident:
		if _, word := _Word(actual.Name); word || !scope._Sees(actual.Name) {
			return nil, false
		}

		named := &workflowpb.Expression_Name{Name: actual.Name}

		return &workflowpb.Expression{Kind: named}, true

	case *syntax.CallExpr:
		held, ok := r._Call(actual, scope)
		if !ok {
			return nil, false
		}

		return &workflowpb.Expression{Kind: &workflowpb.Expression_Call{Call: held}}, true
	}

	return nil, false
}

// _Cases fills a match from the chain testing its local, and reports whether
// every arm of that chain was one it could read.
//
// An else holding one if that compares _match is the next case. An else
// holding anything else is the default.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-30 00:41: reads on a lane, so a case's call may pass a parameter
//   - 2026-10-02 00:04: a case and the default are each a Statement, read in
//     a scope of their own
//   - 2026-10-02 01:16: finds the next case through dialect.NextCase, which
//     the dialect walks a chain with too
func (r *_Reading) _Cases(match *workflowpb.Match, chain *syntax.IfStmt, scope *_Scope) bool {
	value, ok := dialect.Tested(chain.Cond)
	if !ok {
		return false
	}

	taken, ok := r._Branch(chain.True, scope._Inner())
	if !ok {
		return false
	}

	match.Cases = append(match.Cases, &workflowpb.Case{Value: value, Statement: taken})

	if len(chain.False) == 0 {
		return true
	}

	next := dialect.NextCase(chain)
	if next != nil {
		return r._Cases(match, next, scope)
	}

	other, ok := r._Branch(chain.False, scope._Inner())
	if !ok {
		return false
	}

	match.Default = other

	return true
}

// _If is an if, or an elif when elif says so. An else whose statement is
// itself an if is that elif, which is how a chain is stored.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the if node
//   - 2026-10-02 00:04: lifted into graph, taking the generated If
func _If(iff *workflowpb.If, elif bool, prog *_Program) (*syntax.IfStmt, error) {
	if iff == nil || iff.GetThen() == nil {
		return nil, fmt.Errorf("if: %w", ERR_NO_BODY)
	}

	cond, err := _Condition(iff.GetCondition())
	if err != nil {
		return nil, err
	}

	then, err := _Statement(iff.GetThen(), prog)
	if err != nil {
		return nil, err
	}

	node := &syntax.IfStmt{Cond: cond, True: then}
	if elif {
		prog.elif[node] = struct{}{}
	}

	return _Else(node, iff.GetElse(), prog)
}

// _Else sets the other branch. Absent means the script wrote none. An if
// there is an elif.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: attaches the branch to the if
//   - 2026-10-02 00:04: lifted into graph, taking the generated Statement
func _Else(
	node *syntax.IfStmt,
	other *workflowpb.Statement,
	prog *_Program,
) (*syntax.IfStmt, error) {
	if other == nil {
		return node, nil
	}

	if nested := other.GetIf(); nested != nil {
		inner, err := _If(nested, true, prog)
		if err != nil {
			return nil, err
		}

		node.False = []syntax.Stmt{inner}

		return node, nil
	}

	body, err := _Statement(other, prog)
	if err != nil {
		return nil, err
	}

	node.False = body

	return node, nil
}

// _Condition is a bool, a call, or a name.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Condition
func _Condition(cond *workflowpb.Condition) (syntax.Expr, error) {
	if cond == nil {
		return nil, fmt.Errorf("condition: %w", ERR_FORM)
	}

	switch kind := cond.GetKind().(type) {
	case *workflowpb.Condition_Value:
		return _Name(_Bool(kind.Value)), nil

	case *workflowpb.Condition_Name:
		if kind.Name == "" {
			return nil, fmt.Errorf("condition: %w", ERR_FORM)
		}

		return _Name(kind.Name), nil

	case *workflowpb.Condition_Call:
		if kind.Call.GetResult() != "" {
			return nil, fmt.Errorf("%s: %w", kind.Call.GetFunction(), ERR_FORM)
		}

		return _CallExpr(kind.Call)

	default:
		return nil, fmt.Errorf("condition: %w", ERR_FORM)
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
func _Match(match *workflowpb.Match, prog *_Program) ([]syntax.Stmt, error) {
	if match == nil || len(match.GetCases()) == 0 {
		return nil, fmt.Errorf("match: %w", ERR_FORM)
	}

	expr, err := _Expression(match.GetExpression())
	if err != nil {
		return nil, err
	}

	top, err := _Chain(match, prog)
	if err != nil {
		return nil, err
	}

	assign := &syntax.AssignStmt{Op: syntax.EQ, LHS: _Name(SUBJECT), RHS: expr}

	return []syntax.Stmt{assign, top}, nil
}

// _Chain is the comparisons, the first an if and each one after it an elif.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-02 00:04: lifted into graph, taking the generated Match
func _Chain(match *workflowpb.Match, prog *_Program) (*syntax.IfStmt, error) {
	var (
		top   *syntax.IfStmt
		chain *syntax.IfStmt
	)

	first := true

	for _, item := range match.GetCases() {
		arm, err := _Arm(item, prog)
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

	body, err := _Statement(match.GetDefault(), prog)
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
func _Arm(item *workflowpb.Case, prog *_Program) (*syntax.IfStmt, error) {
	if item == nil || item.GetStatement() == nil {
		return nil, fmt.Errorf("match: %w", ERR_NO_BODY)
	}

	body, err := _Statement(item.GetStatement(), prog)
	if err != nil {
		return nil, err
	}

	cond := &syntax.BinaryExpr{
		X:  _Name(SUBJECT),
		Op: syntax.EQL,
		Y:  _Text(strconv.Quote(item.GetValue())),
	}

	return &syntax.IfStmt{Cond: cond, True: body}, nil
}

// _Expression is a string, a call, or a name.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Expression
func _Expression(expr *workflowpb.Expression) (syntax.Expr, error) {
	if expr == nil {
		return nil, fmt.Errorf("expression: %w", ERR_FORM)
	}

	switch kind := expr.GetKind().(type) {
	case *workflowpb.Expression_Value:
		return _Text(strconv.Quote(kind.Value)), nil

	case *workflowpb.Expression_Name:
		if kind.Name == "" {
			return nil, fmt.Errorf("expression: %w", ERR_FORM)
		}

		return _Name(kind.Name), nil

	case *workflowpb.Expression_Call:
		if kind.Call.GetResult() != "" {
			return nil, fmt.Errorf("%s: %w", kind.Call.GetFunction(), ERR_FORM)
		}

		return _CallExpr(kind.Call)

	default:
		return nil, fmt.Errorf("expression: %w", ERR_FORM)
	}
}
