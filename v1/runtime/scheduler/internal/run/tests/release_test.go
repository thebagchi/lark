package tests

import (
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// TestRelease_TokenZeroKeepsTheInheritedMark proves a release of token 0 frees
// nothing.
//
// Without the guard, a compare and swap of 0 for 0 succeeds on locals that
// hold nothing, and the release clears inside, the mark an evaluation inherits
// from the update that started it. That would let the evaluation lock inside
// that update. The test fails with the guard removed.
//
// The state is reached through exported names: the parent takes a lock, a spawn
// started inside it inherits the mark, and a probe running on the spawn's
// thread releases a Hold whose owner is that thread's locals and whose token is 0.
//
// Revisions:
//   - 2026-11-07 23:06: initial creation (C105, B184)
//   - 2026-11-09 01:07: moved to the external package, through Lock and Beside (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestRelease_TokenZeroKeepsTheInheritedMark(t *testing.T) {
	const PARENT = "parent"

	tests := []struct {
		name  string
		token uint64
	}{
		{
			"token zero",
			0,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var (
					parent, _ = _Open(
						t.Context(),
						t,
						nil,
					)
					inside   string
					probeErr error
					probe    = starlark.NewBuiltin(
						"probe",
						func(
							thread *starlark.Thread,
							_ *starlark.Builtin,
							_ starlark.Tuple,
							_ []starlark.Tuple,
						) (starlark.Value, error) {
							owner := thread.Local(run.LOCALS_KEY)

							run.Release(common.Hold{
								Owner: owner,
								Token: tt.token,
							})

							inside, probeErr = run.Updating(thread)

							return starlark.None, nil
						},
					)
				)

				hold, err := run.Lock(
					parent,
					PARENT,
				)
				if err != nil {
					t.Fatalf(
						"lock: %v",
						err,
					)
				}

				defer run.Release(hold)

				handle, err := run.Fork(
					parent,
					probe,
				)
				if err != nil {
					t.Fatalf(
						"fork: %v",
						err,
					)
				}

				if _, err := run.Wait(
					parent,
					handle,
				); err != nil {
					t.Fatalf(
						"wait: %v",
						err,
					)
				}

				if probeErr != nil {
					t.Fatalf(
						"updating: %v",
						probeErr,
					)
				}

				if inside != PARENT {
					t.Fatalf(
						"inside = %q after Release of token %d, want %q",
						inside,
						tt.token,
						PARENT,
					)
				}
			},
		)
	}
}

// TestGuard_C100a_ReleaseTokenZero proves a release of token 0, on locals that
// hold nothing, returns instead of blocking.
//
// Case from Go-level breaker, finding scheduler-F64 (bug B165).
//
// Revisions:
//   - 2026-11-06 00:02: initial creation (C100a)
//   - 2026-11-09 02:09: split out of the table of TestGuard_C100a_ReleaseNil
//     and moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C100a_ReleaseTokenZero(t *testing.T) {
	var (
		thread, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		ended = make(chan struct{})
	)

	go func() {
		defer close(ended)

		run.Release(common.Hold{
			Owner: thread.Local(run.LOCALS_KEY),
			Token: 0,
		})
	}()

	select {
	case <-ended:
	case <-time.After(CANCEL_LIMIT):
		t.Fatal("Release did not return")
	}
}
