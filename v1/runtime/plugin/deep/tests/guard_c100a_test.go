package tests

// This file tests Walk with a nil seen map and Calls with a hand-built syntax
// tree that has a nil name. The parser and IsData never pass either input, so
// the tests call the functions directly (C100a).
//
// Revisions:
//   - 2026-11-08 22:46: states the reason in the first comment (C65b)
//   - 2026-11-10 01:37: moved to tests/ from guard_c100a_internal_test.go and rewrote its
//     header (C120b, Q311, Q322)

import (
	"context"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep/internal/clone"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// TestGuard_C100a_WalkNilSeen proves Walk reads a nil seen map as a new one.
//
// Cases from Go-level breaker, findings state-F5 and F15 (bug B173; F15 is the
// same line through _Every).
//
// Revisions:
//   - 2026-11-06 00:08: initial creation (C100a)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_C100a_WalkNilSeen(t *testing.T) {
	var (
		dict = starlark.NewDict(1)
		err  = dict.SetKey(
			starlark.String("k"),
			starlark.String("v"),
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		value starlark.Value
	}{
		{
			"list",
			starlark.NewList([]starlark.Value{
				starlark.String("x"),
			}),
		},
		{
			"dict",
			dict,
		},
		{
			"set",
			starlark.NewSet(1),
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				bad, err := clone.Walk(
					trail.New(context.Background()),
					tt.value,
					nil,
				)
				if bad != nil || err != nil {
					t.Fatalf(
						"got (%v, %v), want (nil, nil)",
						bad,
						err,
					)
				}
			},
		)
	}
}

// TestGuard_C100a_CallsNil proves Calls finds no match in a nil call or a
// dotted call with no name.
//
// Cases from Go-level breaker, findings state-F10 and F11 (bugs B175 and
// B176).
//
// Revisions:
//   - 2026-11-06 00:08: initial creation (C100a)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_C100a_CallsNil(t *testing.T) {
	tests := []struct {
		name string
		call *syntax.CallExpr
	}{
		{
			"nil call",
			nil,
		},
		{
			"no name",
			&syntax.CallExpr{
				Fn: &syntax.DotExpr{
					X: &syntax.Ident{
						Name: "m",
					},
				},
			},
		},
		{
			"nil dot",
			&syntax.CallExpr{
				Fn: (*syntax.DotExpr)(nil),
			},
		},
		{
			"nil module ident",
			&syntax.CallExpr{
				Fn: &syntax.DotExpr{
					X: (*syntax.Ident)(nil),
					Name: &syntax.Ident{
						Name: "f",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				if clone.Calls(
					tt.call,
					"m",
					"f",
				) {
					t.Fatal("got a match, want none")
				}
			},
		)
	}

	t.Run(
		"match still found",
		func(t *testing.T) {
			call := &syntax.CallExpr{
				Fn: &syntax.DotExpr{
					X: &syntax.Ident{
						Name: "m",
					},
					Name: &syntax.Ident{
						Name: "f",
					},
				},
			}
			if !clone.Calls(
				call,
				"m",
				"f",
			) {
				t.Fatal("got no match, want one")
			}
		},
	)
}
