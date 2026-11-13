package tests

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Told is a Reporter that keeps the name of each line started on a lane.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Told struct {
	mu      sync.Mutex
	started []string
}

// Started keeps the name of a line that calls something.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (r *_Told) Started(
	_ string,
	line *common.Line,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.started = append(
		r.started,
		line.Name,
	)
}

// Ended is not kept: the tests below ask what began.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (r *_Told) Ended(
	_ string,
	_ string,
	_ error,
) {
}

// TestFork_RefusedOnceStopped starts a run, stops it in each way a run is
// stopped, and spawns after. A spawn made once the run is stopped must be
// refused with ErrCancelled and must start no thread: no line is reported for
// it. The control case spawns in a running run and must start one.
//
// Success: the refused cases return ErrCancelled and report only the spine's
// line; the control returns a handle and reports the spawn.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
//   - 2026-11-12 18:58: renamed from TestBeside_RefusedOnceStopped (C129, Q396)
func TestFork_RefusedOnceStopped(t *testing.T) {
	cases := map[string]struct {
		stop func(
			thread *starlark.Thread,
			cancel context.CancelFunc,
		)
		refused bool
		lines   int
	}{
		"running": {
			stop: func(
				*starlark.Thread,
				context.CancelFunc,
			) {
			},
			refused: false,
			lines:   2,
		},
		"a failure ended the run": {
			stop: func(
				thread *starlark.Thread,
				_ context.CancelFunc,
			) {
				_ = scheduler.Fail(
					thread,
					errors.New("broke"),
				)
			},
			refused: true,
			lines:   1,
		},
		"the caller cancelled the context": {
			stop: func(
				_ *starlark.Thread,
				cancel context.CancelFunc,
			) {
				cancel()
			},
			refused: true,
			lines:   1,
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					told        = new(_Told)
					ctx, cancel = context.WithCancel(t.Context())
					target      = starlark.NewBuiltin(
						"later",
						func(
							*starlark.Thread,
							*starlark.Builtin,
							starlark.Tuple,
							[]starlark.Tuple,
						) (starlark.Value, error) {
							return starlark.None, nil
						},
					)
					got error
				)

				defer cancel()

				_, _ = scheduler.Eval(
					ctx,
					"main",
					func(thread *starlark.Thread) (starlark.Value, error) {
						tc.stop(
							thread,
							cancel,
						)

						handle, err := scheduler.Fork(
							thread,
							target,
						)
						got = err

						if handle != nil {
							_, _ = scheduler.Wait(
								thread,
								handle,
							)
						}

						return starlark.None, nil
					},
					&common.Settings{
						Reporter: told,
					},
				)

				if tc.refused != errors.Is(
					got,
					scheduler.ErrCancelled,
				) {
					t.Fatalf(
						"Fork returned %v, want refused=%v",
						got,
						tc.refused,
					)
				}

				if !tc.refused && got != nil {
					t.Fatalf(
						"Fork returned %v, want nil",
						got,
					)
				}

				if len(told.started) != tc.lines {
					t.Fatalf(
						"lines started %q, want %d",
						told.started,
						tc.lines,
					)
				}
			},
		)
	}
}
