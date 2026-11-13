package tests

import (
	"errors"
	"runtime"
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

// TestShowsCode_NamesAFunctionPlainlyShown pins ShowsCode for valid trees, so
// the guards of C87 change nothing for the trees a script makes.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestShowsCode_NamesAFunctionPlainlyShown(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "a def of the file",
			source: "def helper():\n    pass\nstate.set(\"k\", helper)\n",
			want:   "the function helper",
		},
		{
			name:   "a lambda",
			source: "state.set(\"k\", lambda: 1)\n",
			want:   "a lambda",
		},
		{
			name:   "a parenthesised def",
			source: "def helper():\n    pass\nstate.set(\"k\", (helper))\n",
			want:   "the function helper",
		},
		{
			name:   "a value, not a function",
			source: "state.set(\"k\", 1)\n",
			want:   "",
		},
		{
			name:   "another module",
			source: "event.set(\"k\", lambda: 1)\n",
			want:   "",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				file, err := new(syntax.FileOptions).Parse(
					"shows.star",
					test.source,
					0,
				)
				if err != nil {
					t.Fatalf(
						"parse: %v",
						err,
					)
				}

				call, got, failed := deep.ShowsCode(
					file,
					"state",
					"set",
				)
				if failed != nil {
					t.Fatalf(
						"error %v",
						failed,
					)
				}

				if got != test.want || (test.want != "") != (call != nil) {
					t.Fatalf(
						"got %q (call %v), want %q",
						got,
						call != nil,
						test.want,
					)
				}
			},
		)
	}
}

// TestShowsCode_KeepsTheCauseOfAPanic proves a tree that panics inside the walk
// is an error that wraps the recovered panic, so a caller can refuse the
// script and errors.As finds the runtime error. A typed-nil call makes
// the walk dereference nil.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestShowsCode_KeepsTheCauseOfAPanic(t *testing.T) {
	var (
		tree = &syntax.File{
			Stmts: []syntax.Stmt{
				&syntax.ExprStmt{
					X: (*syntax.CallExpr)(nil),
				},
			},
		}
		call, named, err = deep.ShowsCode(
			tree,
			"state",
			"set",
		)
	)
	if err == nil {
		t.Fatal("no error for a tree that panics in the walk")
	}

	if call != nil || named != "" {
		t.Fatalf(
			"found %v %q with an error",
			call,
			named,
		)
	}

	var raised runtime.Error
	if !errors.As(
		err,
		&raised,
	) {
		t.Fatalf(
			"error %v does not wrap the runtime error",
			err,
		)
	}
}
