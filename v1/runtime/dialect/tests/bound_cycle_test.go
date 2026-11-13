package tests

import (
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// TestBound_ASelfContainingTargetEnds hands the exported Bound a tree no parser
// builds: an assignment whose left side is a list that holds itself. Bound
// must return, and bind no name, in place of a stack overflow that no recover
// catches (C108, B196, Q206; breaker finding graph-F32).
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B196)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBound_ASelfContainingTargetEnds(t *testing.T) {
	var (
		list  = new(syntax.ListExpr)
		tuple = new(syntax.TupleExpr)
		paren = new(syntax.ParenExpr)
		into  = make(map[string]bool)
	)

	list.List = append(
		list.List,
		list,
	)
	tuple.List = append(
		tuple.List,
		tuple,
	)
	paren.X = paren

	tests := []struct {
		name  string
		stmts []syntax.Stmt
	}{
		{
			name: "assignment",
			stmts: []syntax.Stmt{
				&syntax.AssignStmt{
					LHS: list,
				},
			},
		},
		{
			name: "tuple holds itself",
			stmts: []syntax.Stmt{
				&syntax.AssignStmt{
					LHS: tuple,
				},
			},
		},
		{
			name: "parenthesis holds itself",
			stmts: []syntax.Stmt{
				&syntax.AssignStmt{
					LHS: paren,
				},
			},
		},
		{
			name: "for variables",
			stmts: []syntax.Stmt{
				&syntax.ForStmt{
					Vars: list,
				},
			},
		},
		{
			name: "cycle beside a name",
			stmts: []syntax.Stmt{
				&syntax.AssignStmt{
					LHS: &syntax.TupleExpr{
						List: []syntax.Expr{
							&syntax.Ident{
								Name: "x",
							},
							list,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				clear(into)

				dialect.Bound(
					tt.stmts,
					into,
				)

				if tt.name == "cycle beside a name" {
					if !into["x"] || len(into) != 1 {
						t.Fatalf(
							"got %v, want only x",
							into,
						)
					}

					return
				}

				if len(into) != 0 {
					t.Fatalf(
						"got %v, want no names",
						into,
					)
				}
			},
		)
	}
}

// TestBound_ABodyThatHoldsItselfEnds hands the exported Bound a statement whose
// own body holds it: an if (both arms), a for and a while. Bound must return
// and keep the names bound beside the cycle, in place of a stack overflow no
// recover catches (C108 fix round 1, B224, Q206, Q65).
//
// Revisions:
//   - 2026-11-08 10:51: initial creation (C108, B224)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBound_ABodyThatHoldsItselfEnds(t *testing.T) {
	var (
		bound = func() syntax.Stmt {
			return &syntax.AssignStmt{
				LHS: &syntax.Ident{
					Name: "x",
				},
			}
		}
		branch = new(syntax.IfStmt)
		other  = new(syntax.IfStmt)
		loop   = new(syntax.ForStmt)
		spin   = new(syntax.WhileStmt)
	)

	branch.True = []syntax.Stmt{bound(), branch}
	other.False = []syntax.Stmt{other, bound()}
	loop.Body = []syntax.Stmt{loop, bound()}
	spin.Body = []syntax.Stmt{bound(), spin}

	tests := []struct {
		name string
		stmt syntax.Stmt
	}{
		{
			name: "if holds itself in True",
			stmt: branch,
		},
		{
			name: "if holds itself in False",
			stmt: other,
		},
		{
			name: "for holds itself",
			stmt: loop,
		},
		{
			name: "while holds itself",
			stmt: spin,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				into := make(map[string]bool)

				dialect.Bound(
					[]syntax.Stmt{tt.stmt},
					into,
				)

				if !into["x"] || len(into) != 1 {
					t.Fatalf(
						"got %v, want only x",
						into,
					)
				}
			},
		)
	}
}
