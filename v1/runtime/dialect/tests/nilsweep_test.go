package tests

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// TestNilSweep_dialect calls every exported function of the package with each
// nil or zero argument a host or a plugin author could pass (C87, Q2, Q85).
// A row passes when the call returns and, where want names a sentinel, the
// error is that sentinel. A panic fails the row.
//
// Revisions:
//   - 2026-11-04 14:00: initial creation (C87)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestNilSweep_dialect(t *testing.T) {
	var (
		nilAssign  *syntax.AssignStmt
		nilBranch  *syntax.BranchStmt
		nilIf      *syntax.IfStmt
		nilBinary  *syntax.BinaryExpr
		nilExprStm *syntax.ExprStmt
	)

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "Bound nil map",
			call: func() error {
				dialect.Bound(
					[]syntax.Stmt{&syntax.AssignStmt{
						LHS: &syntax.Ident{
							Name: "x",
						},
					}},
					nil,
				)

				return nil
			},
		},
		{
			name: "Bound nil statements",
			call: func() error {
				dialect.Bound(
					nil,
					map[string]bool{},
				)

				return nil
			},
		},
		{
			name: "Bound nil statement in list",
			call: func() error {
				dialect.Bound(
					[]syntax.Stmt{nil},
					map[string]bool{},
				)

				return nil
			},
		},
		{
			name: "Bound def with no name",
			call: func() error {
				dialect.Bound(
					[]syntax.Stmt{new(syntax.DefStmt)},
					map[string]bool{},
				)

				return nil
			},
		},
		{
			name: "Bound typed nil statements",
			call: func() error {
				dialect.Bound(
					[]syntax.Stmt{
						(*syntax.DefStmt)(nil),
						nilAssign,
						(*syntax.LoadStmt)(nil),
						(*syntax.ForStmt)(nil),
						(*syntax.WhileStmt)(nil),
						nilIf,
					},
					map[string]bool{},
				)

				return nil
			},
		},
		{
			name: "Bound assignment target typed nil",
			call: func() error {
				dialect.Bound(
					[]syntax.Stmt{
						&syntax.AssignStmt{
							LHS: (*syntax.Ident)(nil),
						},
						&syntax.AssignStmt{
							LHS: &syntax.TupleExpr{
								List: []syntax.Expr{
									nil,
									(*syntax.Ident)(nil),
								},
							},
						},
					},
					map[string]bool{},
				)

				return nil
			},
		},
		{
			name: "Bound load with a nil name",
			call: func() error {
				dialect.Bound(
					[]syntax.Stmt{&syntax.LoadStmt{
						To: []*syntax.Ident{nil},
					}},
					map[string]bool{},
				)

				return nil
			},
		},
		{
			name: "Closed nil",
			call: func() error {
				_ = dialect.Closed(nil)

				return nil
			},
		},
		{
			name: "Closed typed nil closer",
			call: func() error {
				_ = dialect.Closed([]syntax.Stmt{nilExprStm, nilBranch})

				return nil
			},
		},
		{
			name: "Matched nil body",
			call: func() error {
				_, _, _ = dialect.Matched(
					nil,
					0,
				)

				return nil
			},
		},
		{
			name: "Matched negative index",
			call: func() error {
				_, _, ok := dialect.Matched(
					[]syntax.Stmt{nil, nil},
					-1,
				)
				if ok {
					return errors.New("matched")
				}

				return nil
			},
		},
		{
			name: "Matched typed nil statements",
			call: func() error {
				_, _, _ = dialect.Matched(
					[]syntax.Stmt{nilAssign, nilIf},
					0,
				)
				_, _, _ = dialect.Matched(
					[]syntax.Stmt{
						&syntax.AssignStmt{
							Op:  syntax.EQ,
							LHS: (*syntax.Ident)(nil),
						},
						nilIf,
					},
					0,
				)

				return nil
			},
		},
		{
			name: "Matched if with nil condition",
			call: func() error {
				_, _, _ = dialect.Matched(
					[]syntax.Stmt{
						&syntax.AssignStmt{
							Op: syntax.EQ,
							LHS: &syntax.Ident{
								Name: "_match",
							},
						},
						new(syntax.IfStmt),
					},
					0,
				)

				return nil
			},
		},
		{
			name: "NextCase nil chain",
			call: func() error {
				if dialect.NextCase(nil) != nil {
					return errors.New("case")
				}

				return nil
			},
		},
		{
			name: "NextCase typed nil else",
			call: func() error {
				_ = dialect.NextCase(&syntax.IfStmt{
					False: []syntax.Stmt{nilIf},
				})

				return nil
			},
		},
		{
			name: "Tested nil",
			call: func() error {
				_, ok := dialect.Tested(nil)
				if ok {
					return errors.New("tested")
				}

				return nil
			},
		},
		{
			name: "Tested typed nil and zero",
			call: func() error {
				_, _ = dialect.Tested(nilBinary)
				_, _ = dialect.Tested(new(syntax.BinaryExpr))
				_, _ = dialect.Tested(&syntax.BinaryExpr{
					Op: syntax.EQL,
					X:  (*syntax.Ident)(nil),
				})
				_, _ = dialect.Tested(&syntax.BinaryExpr{
					Op: syntax.EQL,
					X: &syntax.Ident{
						Name: "_match",
					},
					Y: (*syntax.Literal)(nil),
				})

				return nil
			},
		},
		{
			name: "Copied nil file",
			call: func() error {
				_, err := dialect.Copied(nil)

				return err
			},
			want: dialect.ErrUncopied,
		},
		{
			name: "Copied empty file",
			call: func() error {
				_, err := dialect.Copied(new(syntax.File))

				return err
			},
		},
		{
			name: "Copied nil statement",
			call: func() error {
				_, err := dialect.Copied(&syntax.File{
					Stmts: []syntax.Stmt{nil},
				})

				return err
			},
			want: dialect.ErrUncopied,
		},
		{
			name: "Copied typed nil statement",
			call: func() error {
				_, err := dialect.Copied(&syntax.File{
					Stmts: []syntax.Stmt{nilAssign},
				})

				return err
			},
			want: dialect.ErrUncopied,
		},
		{
			name: "Copied zero nodes",
			call: func() error {
				_, err := dialect.Copied(&syntax.File{
					Stmts: []syntax.Stmt{
						new(syntax.DefStmt),
						new(syntax.LoadStmt),
						new(syntax.ExprStmt),
					},
				})

				return err
			},
			want: dialect.ErrUncopied,
		},
		{
			name: "Compile nil trees",
			call: func() error {
				_, err := dialect.Compile(
					nil,
					nil,
					nil,
				)

				return err
			},
			want: dialect.ErrUncopied,
		},
		{
			name: "Compile nil compiled tree",
			call: func() error {
				_, err := dialect.Compile(
					new(syntax.File),
					nil,
					starlark.Universe.Has,
				)

				return err
			},
			want: dialect.ErrUncopied,
		},
		{
			name: "Compile nil written tree",
			call: func() error {
				_, err := dialect.Compile(
					nil,
					new(syntax.File),
					starlark.Universe.Has,
				)

				return err
			},
			want: dialect.ErrUncopied,
		},
		{
			name: "Compile nil predeclared",
			call: func() error {
				_, err := dialect.Compile(
					new(syntax.File),
					new(syntax.File),
					nil,
				)

				return err
			},
		},
		{
			name: "Compile zero trees",
			call: func() error {
				_, err := dialect.Compile(
					new(syntax.File),
					new(syntax.File),
					starlark.Universe.Has,
				)

				return err
			},
		},
		{
			name: "Compile typed nil statement",
			call: func() error {
				_, err := dialect.Compile(
					&syntax.File{
						Stmts: []syntax.Stmt{nilExprStm},
					},
					&syntax.File{
						Stmts: []syntax.Stmt{nilExprStm},
					},
					starlark.Universe.Has,
				)

				return err
			},
			want: dialect.ErrUncopied,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					got      error
					panicked any
				)

				func() {
					defer func() {
						panicked = recover()
					}()

					got = test.call()
				}()

				if panicked != nil {
					t.Fatalf(
						"panicked: %v",
						panicked,
					)
				}

				if test.want != nil && !errors.Is(
					got,
					test.want,
				) {
					t.Fatalf(
						"error %v, want %v",
						got,
						test.want,
					)
				}
			},
		)
	}
}
