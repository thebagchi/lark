package tests

import (
	"errors"
	"runtime"
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/event"
)

// TestCheck_RefusesASourceItCannotRead proves the plugin's Check returns an
// error that wraps the cause when the walk of the tree panics, in place of
// passing the script (C87 fix round 1).
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCheck_RefusesASourceItCannotRead(t *testing.T) {
	for _, registered := range plugin.Registered() {
		if registered.Name() != "event" {
			continue
		}

		checker, ok := registered.(common.Checking)
		if !ok {
			t.Fatal("not a Checking")
		}

		err := checker.Check(&syntax.File{
			Stmts: []syntax.Stmt{
				&syntax.ExprStmt{
					X: (*syntax.CallExpr)(nil),
				},
			},
		})
		if err == nil {
			t.Fatal("a tree that cannot be read passed")
		}

		var raised runtime.Error
		if !errors.As(
			err,
			&raised,
		) {
			t.Fatalf(
				"error %v does not wrap the cause",
				err,
			)
		}

		return
	}

	t.Fatal("plugin not registered")
}
