package tests

// This file tests Descent against a nil receiver and Failure, Handles and a nil
// function against a nil receiver, a nil function, a nil handle and a value
// whose String panics. No script can build these inputs, so the tests call the
// functions directly.
//
// Revisions:
//   - 2026-11-04 18:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from guard_nil_internal_test.go and rewrote its
//     header (C120b, Q311, Q322)

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Panicking is a value whose String panics.
//
// Revisions:
//   - 2026-11-04 18:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Panicking struct {
	starlark.Value
}

// String panics.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (p *_Panicking) String() string {
	panic("panic in String")
}

// TestGuard_DescentNilReceiver proves the walk methods of Descent answer a nil
// receiver and a nil method, with scheduler.ErrRun and not a panic.
//
// Cases from Go-level breaker run 3, findings scheduler-F28, F29 and F30 (bugs
// B080, B081, B082). _Tuple and _Receiver moved onto Descent in C52; F29 (a nil
// path) and F30 (a nil method) pass on the base through C52's _Path and
// _Receiver guards.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_DescentNilReceiver(t *testing.T) {
	var nobody *builtin.Descent

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			"tuple of a nil receiver",
			func() error {
				return nobody.Tuple(starlark.Tuple{})
			},
			scheduler.ErrRun,
		},
		{
			"tuple with a nil path",
			func() error {
				return (*builtin.Descent)(&builtin.Reach{}).Tuple(starlark.Tuple{})
			},
			nil,
		},
		{
			"receiver of a nil method",
			func() error {
				return (*builtin.Descent)(new(builtin.Reach)).Receiver(nil)
			},
			scheduler.ErrRun,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := test.call()
				if !errors.Is(
					err,
					test.want,
				) || (test.want == nil && err != nil) {
					t.Fatalf(
						"got %v, want %v",
						err,
						test.want,
					)
				}
			},
		)
	}
}

// TestGuard_Failure proves a message whose String panics, and a nil message,
// both give ErrAssert.
//
// Case from Go-level breaker run 3, finding scheduler-F44 (bug B095). F45 (bug
// B096) has no test: its race is inside the String of the breaker's own value,
// which writes a counter with no lock, so no code in Failure could fix it.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_Failure(t *testing.T) {
	tests := []struct {
		name string
		msg  starlark.Value
	}{
		{
			"nil message",
			nil,
		},
		{
			"message whose String panics",
			&_Panicking{},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := builtin.Failure(test.msg)
				if !errors.Is(
					err,
					core.ErrAssert,
				) {
					t.Fatalf(
						"got %v, want ErrAssert",
						err,
					)
				}
			},
		)
	}
}

// TestGuard_NilHandles proves a nil handle, or a nil argument where a handle is
// wanted, is ErrHandle and not a panic, in Waited, Handles and Abort.
//
// Cases from Go-level breaker run 3, findings scheduler-F46, F48 and F49 (bugs
// B097, B099, B100). Abort has no error to return: a nil handle is skipped.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-07 20:07: uses common.Handle (C91w5, Q127)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrHandle, formerly ErrNotAHandle (C121, Q348)
func TestGuard_NilHandles(t *testing.T) {
	t.Run(
		"_Waited",
		func(t *testing.T) {
			_, err := builtin.Waited(
				new(starlark.Thread),
				[]common.Handle{nil},
			)
			if !errors.Is(
				err,
				core.ErrHandle,
			) {
				t.Fatalf(
					"got %v, want ErrHandle",
					err,
				)
			}
		},
	)

	t.Run(
		"_Waited aborts the handles after a nil one",
		func(t *testing.T) {
			_, err := builtin.Waited(
				new(starlark.Thread),
				[]common.Handle{
					nil,
					nil,
				},
			)
			if !errors.Is(
				err,
				core.ErrHandle,
			) {
				t.Fatalf(
					"got %v, want ErrHandle",
					err,
				)
			}
		},
	)

	t.Run(
		"_Handles",
		func(t *testing.T) {
			_, err := builtin.Handles(
				"test_builtin",
				starlark.Tuple{nil},
				nil,
			)
			if !errors.Is(
				err,
				core.ErrHandle,
			) {
				t.Fatalf(
					"got %v, want ErrHandle",
					err,
				)
			}
		},
	)

	t.Run(
		"_Abort",
		func(t *testing.T) {
			builtin.Abort(
				nil,
				[]common.Handle{nil},
			)
		},
	)
}

// TestGuard_NilFunction proves Defaulted and Uncaptured take a nil function
// without a panic, and that a spawn of one is an error.
//
// Cases from Go-level breaker run 3, findings scheduler-F50, F51 and F52 (bugs
// B101, B102, B103). F50 passes on the base through C52's nil guard in the walk.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_NilFunction(t *testing.T) {
	var nothing *starlark.Function

	t.Run(
		"_Defaulted takes no parameter, so all have defaults",
		func(t *testing.T) {
			if !builtin.Defaulted(nothing) {
				t.Fatal("a nil function has a parameter without a default")
			}
		},
	)

	t.Run(
		"_Uncaptured",
		func(t *testing.T) {
			err := builtin.Uncaptured(nothing)
			if !errors.Is(
				err,
				scheduler.ErrRun,
			) {
				t.Fatalf(
					"got %v, want scheduler.ErrRun",
					err,
				)
			}
		},
	)

	t.Run(
		"_Started",
		func(t *testing.T) {
			_, err := builtin.Started(
				nil,
				nothing,
				"test",
				"test",
			)
			if !errors.Is(
				err,
				scheduler.ErrRun,
			) {
				t.Fatalf(
					"got %v, want scheduler.ErrRun",
					err,
				)
			}
		},
	)
}
