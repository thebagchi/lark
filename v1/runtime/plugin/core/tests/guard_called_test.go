package tests

// This file tests that Lined and Paused do not panic when their builtin is nil.
// No script can build that input, because the wrapper of NewBuiltin always
// passes its own builtin, so the tests call them directly (C101, C102).
//
// Revisions:
//   - 2026-11-06 21:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from guard_called_internal_test.go and rewrote its
//     header (C120b, Q311, Q322)

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestGuard_CalledNilBuiltin proves join and sleep name a nil builtin
// "builtin" in their error, and do not panic.
//
// Case from the C100b tester (bug B182).
//
// Revisions:
//   - 2026-11-06 06:45: initial creation (C101)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_CalledNilBuiltin(t *testing.T) {
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{
			"join with a keyword argument",
			func() error {
				_, err := builtin.Lined(builtin.Waited)(
					nil,
					nil,
					starlark.Tuple{},
					[]starlark.Tuple{{starlark.String("k"), starlark.None}},
				)

				return err
			},
			"builtin takes no keyword arguments",
		},
		{
			"sleep outside a run",
			func() error {
				_, err := builtin.Paused(
					nil,
					nil,
					0,
				)

				return err
			},
			"builtin: ",
		},
		{
			"sleep in a cancelled run",
			func() error {
				var (
					ctx, cancel = context.WithCancel(context.Background())
					thread      = new(starlark.Thread)
					finish      = scheduler.Begin(
						ctx,
						thread,
						"guard",
						nil,
					)
				)
				cancel()

				_, err := builtin.Paused(
					thread,
					nil,
					time.Hour,
				)

				_ = finish(nil)

				return err
			},
			"builtin: ",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := test.call()
				if err == nil || !strings.HasPrefix(
					err.Error(),
					test.want,
				) {
					t.Fatalf(
						"got %v, want prefix %q",
						err,
						test.want,
					)
				}
			},
		)
	}
}
