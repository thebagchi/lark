package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

// TestNilSweep_deep calls every exported function of the package with each
// nil or zero argument a host or a plugin author could pass (C87, Q2, Q85).
// A row passes when the call returns before the timeout and, where want names
// a sentinel, the error is that sentinel. A panic or a hang fails the row.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestNilSweep_deep(t *testing.T) {
	var (
		nilCtx   context.Context
		nilList  *starlark.List
		nilDict  *starlark.Dict
		nilSet   *starlark.Set
		nilTuple starlark.Tuple
		nilCall  *syntax.CallExpr
		nilDef   *syntax.DefStmt
	)

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "Copy nil value",
			call: func() error {
				_, err := deep.Copy(
					context.Background(),
					nil,
				)

				return err
			},
		},
		{
			name: "Copy nil context",
			call: func() error {
				_, err := deep.Copy(
					nilCtx,
					_Held(starlark.MakeInt(1)),
				)

				return err
			},
		},
		{
			name: "Copy typed nil containers",
			call: func() error {
				for _, value := range []starlark.Value{nilList, nilDict, nilSet} {
					_, err := deep.Copy(
						context.Background(),
						value,
					)
					if err != nil {
						return err
					}
				}

				return nil
			},
		},
		{
			name: "Copy zero containers",
			call: func() error {
				for _, value := range []starlark.Value{
					new(starlark.List),
					new(starlark.Dict),
					new(starlark.Set),
					nilTuple,
				} {
					_, err := deep.Copy(
						context.Background(),
						value,
					)
					if err != nil {
						return err
					}
				}

				return nil
			},
		},
		{
			name: "Copy list holding a nil value",
			call: func() error {
				_, err := deep.Copy(
					context.Background(),
					starlark.NewList([]starlark.Value{nil}),
				)

				return err
			},
		},
		{
			name: "Copy tuple holding a nil value",
			call: func() error {
				_, err := deep.Copy(
					context.Background(),
					starlark.Tuple{nil, nilList},
				)

				return err
			},
		},
		{
			name: "Copy list holding a typed nil list",
			call: func() error {
				_, err := deep.Copy(
					context.Background(),
					_Held(
						nilList,
						nilDict,
						nilSet,
					),
				)

				return err
			},
		},
		{
			name: "IsData nil value",
			call: func() error {
				_, _, err := deep.IsData(
					context.Background(),
					nil,
				)

				return err
			},
		},
		{
			name: "IsData nil context",
			call: func() error {
				_, _, err := deep.IsData(
					nilCtx,
					_Held(starlark.MakeInt(1)),
				)

				return err
			},
		},
		{
			name: "IsData typed nil containers",
			call: func() error {
				for _, value := range []starlark.Value{nilList, nilDict, nilSet} {
					_, _, err := deep.IsData(
						context.Background(),
						value,
					)
					if err != nil {
						return err
					}
				}

				return nil
			},
		},
		{
			name: "IsData zero containers",
			call: func() error {
				for _, value := range []starlark.Value{
					new(starlark.List),
					new(starlark.Dict),
					new(starlark.Set),
					nilTuple,
				} {
					_, _, err := deep.IsData(
						context.Background(),
						value,
					)
					if err != nil {
						return err
					}
				}

				return nil
			},
		},
		{
			name: "IsData containers holding a nil value",
			call: func() error {
				for _, value := range []starlark.Value{
					starlark.NewList([]starlark.Value{nil}),
					starlark.Tuple{nil},
					starlark.NewList([]starlark.Value{nilList}),
				} {
					_, _, err := deep.IsData(
						context.Background(),
						value,
					)
					if err != nil {
						return err
					}
				}

				return nil
			},
		},
		{
			name: "ShowsCode nil tree",
			call: func() error {
				call, named, err := deep.ShowsCode(
					nil,
					"state",
					"set",
				)
				if call != nil || named != "" {
					return errors.New("found")
				}

				return err
			},
		},
		{
			name: "ShowsCode empty tree, empty names",
			call: func() error {
				_, _, err := deep.ShowsCode(
					new(syntax.File),
					"",
					"",
				)

				return err
			},
		},
		{
			name: "ShowsCode nil and typed nil statements",
			call: func() error {
				stmts := []syntax.Stmt{
					nil,
					nilDef,
					new(syntax.DefStmt),
				}

				return _ShowsErr(&syntax.File{
					Stmts: stmts,
				})
			},
		},
		{
			name: "ShowsCode calls with missing parts",
			call: func() error {
				tree := &syntax.File{
					Stmts: []syntax.Stmt{
						&syntax.ExprStmt{
							X: nilCall,
						},
						&syntax.ExprStmt{
							X: &syntax.CallExpr{
								Fn:   &syntax.DotExpr{},
								Args: []syntax.Expr{nil, nil},
							},
						},
						_StateSet(
							&syntax.Ident{
								Name: "state",
							},
							nil,
						),
						_StateSet(
							(*syntax.Ident)(nil),
							&syntax.ParenExpr{},
						),
						_StateSet(
							&syntax.Ident{
								Name: "state",
							},
							(*syntax.ParenExpr)(nil),
						),
						_StateSet(
							&syntax.Ident{
								Name: "state",
							},
							(*syntax.Ident)(nil),
						),
					},
				}

				return _ShowsErr(tree)
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					got      error
					panicked any
					finished = make(chan struct{})
				)

				go func() {
					defer close(finished)
					defer func() {
						panicked = recover()
					}()

					got = test.call()
				}()

				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Fatal("hung")
				}

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

// _StateSet is the statement state.set(<nil>, arg) with the module part given,
// to build a call with a part missing.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _StateSet(
	module syntax.Expr,
	arg syntax.Expr,
) syntax.Stmt {
	return &syntax.ExprStmt{
		X: &syntax.CallExpr{
			Fn: &syntax.DotExpr{
				X: module,
				Name: &syntax.Ident{
					Name: "set",
				},
			},
			Args: []syntax.Expr{nil, arg},
		},
	}
}

// _ShowsErr is the error of ShowsCode for state.set on tree.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _ShowsErr(tree *syntax.File) error {
	_, _, err := deep.ShowsCode(
		tree,
		"state",
		"set",
	)

	return err
}
