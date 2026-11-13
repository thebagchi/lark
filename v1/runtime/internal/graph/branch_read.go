package graph

import (
	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// One direction of one subject lives here: reading an if and a match out of a
// script. A branch is one statement; an elif is an If held as the else of
// another; a match is an assignment to _match and the chain that compares it.
// The other direction is in branch_write.go.

// If is the statement an if is, and whether it models.
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
//   - 2026-11-02 10:25: moved to branch_read.go
//   - 2026-11-06 22:02: a nil if does not model (C100c, B152)
//   - 2026-11-10 00:25: renamed from _If, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) If(
	stmt *syntax.IfStmt,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	if stmt == nil {
		return nil, false
	}

	condition, ok := r.Condition(
		stmt.Cond,
		scope,
	)
	if !ok {
		return nil, false
	}

	then, ok := r._Branch(
		stmt.True,
		scope.Inner(),
	)
	if !ok {
		return nil, false
	}

	branch := &workflowpb.If{
		Condition: condition,
		Then:      then,
	}

	if len(stmt.False) > 0 {
		branch.Else, ok = r._Branch(
			stmt.False,
			scope.Inner(),
		)
		if !ok {
			return nil, false
		}
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_If{
			If: branch,
		},
	}, true
}

// Condition is the Condition an expression states, or false.
//
// True or False, a call of a listed function, or a name the scope sees. A
// comparison, a negation or anything else states no condition this schema can
// carry.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-30 00:41: reads on a lane, so a condition may pass a parameter
//   - 2026-10-02 00:04: a name is a condition, and the call is read in a scope
//   - 2026-11-02 10:25: moved to branch_read.go
//   - 2026-11-10 00:25: renamed from _Condition, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-13 10:16: reads a bool from a workflow.Value (C132, Q425)
func (r *Reading) Condition(
	expr syntax.Expr,
	scope *Scope,
) (*workflowpb.Condition, bool) {
	switch actual := expr.(type) {
	case *syntax.Ident:
		value, word := _Word(actual.Name)
		if word {
			flag, ok := value.GetKind().(*workflowpb.Value_Bool)
			if !ok {
				return nil, false
			}

			return &workflowpb.Condition{
				Kind: &workflowpb.Condition_Value{
					Value: flag.Bool,
				},
			}, true
		}

		if !scope.Sees(actual.Name) {
			return nil, false
		}

		named := &workflowpb.Condition_Name{
			Name: actual.Name,
		}

		return &workflowpb.Condition{
			Kind: named,
		}, true

	case *syntax.CallExpr:
		held, ok := r.Call(
			actual,
			scope,
		)
		if !ok {
			return nil, false
		}

		return &workflowpb.Condition{
			Kind: &workflowpb.Condition_Call{
				Call: held,
			},
		}, true
	}

	return nil, false
}

// Expression is the Expression a match evaluates, or false: a string, a call
// of a listed function, or a name the scope sees.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-02 10:25: moved to branch_read.go
//   - 2026-11-10 00:25: renamed from _Expression, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-12 23:35: a bytes literal is no string to match (C130, B290, Q403)
func (r *Reading) Expression(
	expr syntax.Expr,
	scope *Scope,
) (*workflowpb.Expression, bool) {
	switch actual := expr.(type) {
	case *syntax.Literal:
		text, ok := actual.Value.(string)
		if !ok || actual.Token == syntax.BYTES {
			return nil, false
		}

		value := &workflowpb.Expression_Value{
			Value: text,
		}

		return &workflowpb.Expression{
			Kind: value,
		}, true

	case *syntax.Ident:
		if _, word := _Word(actual.Name); word || !scope.Sees(actual.Name) {
			return nil, false
		}

		named := &workflowpb.Expression_Name{
			Name: actual.Name,
		}

		return &workflowpb.Expression{
			Kind: named,
		}, true

	case *syntax.CallExpr:
		held, ok := r.Call(
			actual,
			scope,
		)
		if !ok {
			return nil, false
		}

		return &workflowpb.Expression{
			Kind: &workflowpb.Expression_Call{
				Call: held,
			},
		}, true
	}

	return nil, false
}

// Cases fills a match from the chain testing its local, and reports whether
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
//   - 2026-11-02 10:25: moved to branch_read.go
//   - 2026-11-06 22:02: a nil chain is not read (C100c, B149)
//   - 2026-11-10 00:25: renamed from _Cases, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Cases(
	match *workflowpb.Match,
	chain *syntax.IfStmt,
	scope *Scope,
) bool {
	if chain == nil {
		return false
	}

	value, ok := dialect.Tested(chain.Cond)
	if !ok {
		return false
	}

	taken, ok := r._Branch(
		chain.True,
		scope.Inner(),
	)
	if !ok {
		return false
	}

	match.Cases = append(
		match.Cases,
		&workflowpb.Case{
			Value:     value,
			Statement: taken,
		},
	)

	if len(chain.False) == 0 {
		return true
	}

	next := dialect.NextCase(chain)
	if next != nil {
		return r.Cases(
			match,
			next,
			scope,
		)
	}

	other, ok := r._Branch(
		chain.False,
		scope.Inner(),
	)
	if !ok {
		return false
	}

	match.Default = other

	return true
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
//   - 2026-11-02 10:25: moved to branch_read.go
//   - 2026-11-10 01:00: now uses Reading, Scope, were _Reading, _Scope (C120a, Q314)
func (r *Reading) _Branch(
	stmts []syntax.Stmt,
	scope *Scope,
) (*workflowpb.Statement, bool) {
	body := dialect.Closed(stmts)
	if len(body) != 1 {
		return nil, false
	}

	return r._Statement(
		body[0],
		scope,
	)
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
//   - 2026-11-02 10:25: moved to branch_read.go
//   - 2026-11-10 01:00: now uses Reading, Scope, Expression, Cases, were _Reading, _Scope,
//     _Expression, _Cases (C120a, Q314)
func (r *Reading) _Matched(
	body []syntax.Stmt,
	idx int,
	scope *Scope,
) *workflowpb.Statement {
	assign, chain, ok := dialect.Matched(
		body,
		idx,
	)
	if !ok {
		return nil
	}

	expression, ok := r.Expression(
		assign.RHS,
		scope,
	)
	if !ok {
		return nil
	}

	match := &workflowpb.Match{
		Expression: expression,
	}

	if !r.Cases(
		match,
		chain,
		scope,
	) {
		return nil
	}

	return &workflowpb.Statement{
		Action: &workflowpb.Statement_Match{
			Match: match,
		},
	}
}
