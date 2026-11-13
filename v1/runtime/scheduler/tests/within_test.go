package tests

import (
	"context"
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Target is the function a test case runs as the target of Within.
//
// Revisions:
//   - 2026-11-02 01:08: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Target func(*starlark.Thread) (starlark.Value, error)

// TestWithin_Results proves Within returns what its target produced: a value,
// the error it raised, a failure Fail kept, a cancellation and a panic.
//
// Revisions:
//   - 2026-11-02 00:50: initial creation
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses ForkOption, formerly BesideOption (C129, Q396)
func TestWithin_Results(t *testing.T) {
	var (
		raised = errors.New("raised")
		failed = errors.New("failed")
	)

	cases := map[string]struct {
		body      func(context.CancelFunc) _Target
		opts      []common.ForkOption
		wantValue starlark.Value
		wantIs    error
	}{
		"a value": {
			body: func(context.CancelFunc) _Target {
				return func(*starlark.Thread) (starlark.Value, error) {
					return starlark.MakeInt(7), nil
				}
			},
			wantValue: starlark.MakeInt(7),
		},
		"an error the target raised": {
			body: func(context.CancelFunc) _Target {
				return func(*starlark.Thread) (starlark.Value, error) {
					return nil, raised
				}
			},
			wantIs: raised,
		},
		"a failure Fail kept, with nothing returned": {
			body: func(context.CancelFunc) _Target {
				return func(thread *starlark.Thread) (starlark.Value, error) {
					_ = scheduler.Fail(
						thread,
						failed,
					)

					return starlark.MakeInt(1), nil
				}
			},
			opts:   []common.ForkOption{scheduler.Catching()},
			wantIs: failed,
		},
		"a failure under a cancel": {
			body: func(cancel context.CancelFunc) _Target {
				return func(*starlark.Thread) (starlark.Value, error) {
					cancel()

					return nil, raised
				}
			},
			wantIs: scheduler.ErrCancelled,
		},
		"a value under a cancel": {
			body: func(cancel context.CancelFunc) _Target {
				return func(*starlark.Thread) (starlark.Value, error) {
					cancel()

					return starlark.MakeInt(3), nil
				}
			},
			wantValue: starlark.MakeInt(3),
		},
		"a panic, which ends the run and so reads as cancelled": {
			body: func(context.CancelFunc) _Target {
				return func(*starlark.Thread) (starlark.Value, error) {
					panic("boom")
				}
			},
			wantIs: scheduler.ErrCancelled,
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					ctx, cancel = context.WithCancel(t.Context())
					thread      = &starlark.Thread{}
					finish      = scheduler.Begin(
						ctx,
						thread,
						common.ENTRY,
						nil,
					)
					run = tc.body(cancel)
				)

				defer cancel()
				defer func() {
					_ = finish(nil)
				}()

				got, err := scheduler.Within(
					thread,
					starlark.NewBuiltin(
						"target",
						func(
							th *starlark.Thread,
							_ *starlark.Builtin,
							_ starlark.Tuple,
							_ []starlark.Tuple,
						) (starlark.Value, error) {
							return run(th)
						},
					),
					tc.opts...,
				)

				if tc.wantIs != nil && !errors.Is(
					err,
					tc.wantIs,
				) {
					t.Fatalf(
						"got error %v, want %v",
						err,
						tc.wantIs,
					)
				}

				if tc.wantIs == nil && err != nil {
					t.Fatalf(
						"got error %v, want none",
						err,
					)
				}

				if tc.wantValue != nil && got != tc.wantValue {
					t.Fatalf(
						"got value %v, want %v",
						got,
						tc.wantValue,
					)
				}

				if tc.wantValue == nil && got != nil {
					t.Fatalf(
						"got value %v, want nil",
						got,
					)
				}
			},
		)
	}
}

// TestWithin_RefusesAThreadWithNoRun proves Within returns ErrRun and no
// value when the thread carries no run.
//
// Revisions:
//   - 2026-11-02 00:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestWithin_RefusesAThreadWithNoRun(t *testing.T) {
	got, err := scheduler.Within(
		new(starlark.Thread),
		starlark.NewBuiltin(
			"x",
			nil,
		),
	)
	if !errors.Is(
		err,
		scheduler.ErrRun,
	) || got != nil {
		t.Fatalf(
			"got %v, %v, want nil and ErrRun",
			got,
			err,
		)
	}
}
