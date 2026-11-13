// This file tests the guards of Produced, Calling, Detach and Take against a nil
// context, a nil builtin or a nil handle, which no script can build. These are
// breaker guard tests (Q206, Q207).
//
// Revisions:
//   - 2026-11-03 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:45: the header names the Q206 and Q207 decisions (C65c)
//   - 2026-11-10 00:25: moved to tests/ from guard_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
package tests

import (
	"context"
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// NO_CONTEXT is a nil context. A literal nil in a call is a lint finding, and a
// variable is the same value.
//
// Revisions:
//   - 2026-11-03 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	NO_CONTEXT context.Context
)

// TestGuard_Produced proves a nil context is read as "nothing says the run was
// stopped", so the error comes back as it is.
//
// Case from Go-level breaker run 3, finding scheduler-F6 (bug B061).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_Produced(t *testing.T) {
	var (
		failure = errors.New("failure")
		tests   = []struct {
			name    string
			outcome error
			err     error
			want    error
		}{
			{
				"nil context, an error",
				nil,
				failure,
				failure,
			},
			{
				"nil context, no error",
				nil,
				nil,
				nil,
			},
			{
				"nil context, an outcome",
				failure,
				nil,
				failure,
			},
		}
	)

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				got := run.Produced(
					NO_CONTEXT,
					test.outcome,
					test.err,
				)
				if !errors.Is(
					got,
					test.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						got,
						test.want,
					)
				}
			},
		)
	}
}

// TestGuard_Calling proves a nil builtin with nothing to call is the
// ErrRun refusal and not a panic.
//
// Case from Go-level breaker run 3, finding scheduler-F17 (bug B072).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_Calling(t *testing.T) {
	tests := []struct {
		name string
		fn   *starlark.Builtin
	}{
		{
			"nil builtin",
			nil,
		},
		{
			"named builtin",
			starlark.NewBuiltin(
				"call",
				nil,
			),
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := run.Calling(
					nil,
					test.fn,
					starlark.Tuple{},
					nil,
				)
				if !errors.Is(
					err,
					run.ErrRun,
				) {
					t.Fatalf(
						"got %v, want ErrRun",
						err,
					)
				}
			},
		)
	}
}

// TestGuard_Detach proves a nil handle detaches to a stop that does nothing.
//
// Case from Go-level breaker run 3, finding scheduler-F18 (bug B073).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_Detach(t *testing.T) {
	var handle *run.Handle

	stop := handle.Detach()
	if stop == nil {
		t.Fatal("got a nil stop")
	}

	stop()
}

// TestGuard_Take proves a nil context with a full slot is ErrRun and not a
// panic, and that a free slot is still taken with a nil context.
//
// Case from Go-level breaker run 3, finding scheduler-F19 (bug B074).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_Take(t *testing.T) {
	t.Run(
		"full slot",
		func(t *testing.T) {
			slot := make(
				chan struct{},
				1,
			)
			slot <- struct{}{}

			err := run.Take(
				NO_CONTEXT,
				slot,
				"name",
			)
			if !errors.Is(
				err,
				run.ErrRun,
			) {
				t.Fatalf(
					"got %v, want ErrRun",
					err,
				)
			}
		},
	)

	t.Run(
		"free slot",
		func(t *testing.T) {
			var (
				slot = make(
					chan struct{},
					1,
				)
				err = run.Take(
					NO_CONTEXT,
					slot,
					"name",
				)
			)
			if err != nil {
				t.Fatalf(
					"got %v, want nil",
					err,
				)
			}
		},
	)
}
