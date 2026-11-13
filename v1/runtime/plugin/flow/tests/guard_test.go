package tests

// This file tests the Go-level breaker findings flow-F2, F7, F8, F15 to F23
// (C100b): TimeoutArgs, the builtin name of Repeat, the methods of Attempts,
// CountDelay, RetryFail, GaveUp and TimeoutRun against a nil or hand-built
// value that a script cannot make. Only a Go caller can pass those, so the
// tests call the functions directly.
//
// Revisions:
//   - 2026-11-06 06:27: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from guard_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/flow/internal/attempt"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestGuard_NilBuiltinHasAName proves a nil builtin names itself ANONYMOUS in
// place of panicking (B130, B142).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_NilBuiltinHasAName(t *testing.T) {
	tests := []struct {
		name string
		call func(thread *starlark.Thread) error
	}{
		{
			name: "timeout args",
			call: func(thread *starlark.Thread) error {
				_, _, _, err := attempt.TimeoutArgs(
					nil,
					nil,
					nil,
				)

				return err
			},
		},
		{
			name: "repeat",
			call: func(thread *starlark.Thread) error {
				_, err := attempt.Repeat(
					thread,
					nil,
					nil,
					nil,
				)

				return err
			},
		},
		{
			name: "retry",
			call: func(thread *starlark.Thread) error {
				_, err := attempt.Retry(
					thread,
					nil,
					nil,
					nil,
				)

				return err
			},
		},
		{
			name: "timeout",
			call: func(thread *starlark.Thread) error {
				_, err := attempt.Timeout(
					thread,
					nil,
					nil,
					nil,
				)

				return err
			},
		},
		{
			name: "attempt",
			call: func(thread *starlark.Thread) error {
				_, err := attempt.Attempt(
					thread,
					nil,
					starlark.Tuple{starlark.None},
					nil,
				)

				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := test.call(&starlark.Thread{
					Name: "t",
				})
				if err == nil {
					t.Fatal("want an error, got nil")
				}

				if !strings.Contains(
					err.Error(),
					"builtin",
				) {
					t.Fatalf(
						"error %q does not name the builtin",
						err,
					)
				}
			},
		)
	}
}

// TestGuard_NilAttempts proves a nil *Attempts refuses with
// scheduler.ErrRun in each method that reads it (B133, B141, B143, B144).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_NilAttempts(t *testing.T) {
	var (
		attempts *attempt.Attempts
		thread   *starlark.Thread
	)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "pause",
			call: func() error {
				return attempts.Pause(
					thread,
					2,
				)
			},
		},
		{
			name: "within",
			call: func() error {
				_, err := attempts.Within(
					thread,
					1,
				)

				return err
			},
		},
		{
			name: "retry fail",
			call: func() error {
				_, err := attempt.RetryFail(
					thread,
					attempts,
					1,
					errors.New("fail"),
				)

				return err
			},
		},
		{
			name: "gave up",
			call: func() error {
				_, err := attempt.GaveUp(
					thread,
					attempts,
					nil,
				)

				return err
			},
		},
		{
			name: "retried",
			call: func() error {
				_, err := attempt.Retried(
					thread,
					attempts,
				)

				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := test.call()
				if !errors.Is(
					err,
					scheduler.ErrRun,
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

// TestGuard_CountDelayGiven proves CountDelay refuses a nil Given and
// arguments shorter than WRAPPED (B139, B140).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_CountDelayGiven(t *testing.T) {
	tests := []struct {
		name  string
		given *attempt.Given
	}{
		{
			name:  "nil given",
			given: nil,
		},
		{
			name:  "nil args",
			given: &attempt.Given{},
		},
		{
			name: "one argument",
			given: &attempt.Given{
				Args: starlark.Tuple{starlark.None},
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := attempt.CountDelay(
					"test",
					test.given,
				)
				if !errors.Is(
					err,
					scheduler.ErrRun,
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

// TestGuard_TimeoutRunLimit proves TimeoutRun refuses a nil limit and a limit
// with no target, before it opens a line (B145, B146).
//
// Revisions:
//   - 2026-11-06 00:05: initial creation (C100b)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_TimeoutRunLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit *attempt.Limit
	}{
		{
			name:  "nil limit",
			limit: nil,
		},
		{
			name: "nil target",
			limit: &attempt.Limit{
				Callee: "test",
				Given:  starlark.String("foo"),
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := attempt.TimeoutRun(
					&starlark.Thread{
						Name: "t",
					},
					test.limit,
				)
				if !errors.Is(
					err,
					scheduler.ErrRun,
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
