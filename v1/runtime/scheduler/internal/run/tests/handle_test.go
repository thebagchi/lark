package tests

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

const (
	// WORKER_THREAD is the id the first spawn of the spine takes, and UNHASHABLE
	// is the word the refusal to hash carries.
	//
	// Revisions:
	//   - 2026-11-08 22:45: initial creation, as the external form of the constants in
	//     handle_internal_test.go (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WORKER_THREAD = "thread_1"
	UNHASHABLE    = "unhashable"
)

// _Spawned returns the handle of a spawn of WORKER that has ended, as a script
// holds one.
//
// Revisions:
//   - 2026-11-08 22:45: initial creation, as the external form of _Handle in
//     handle_internal_test.go, which set the fields without spawning (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func _Spawned(t *testing.T) common.Handle {
	t.Helper()

	var (
		thread, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		handle, err = run.Fork(
			thread,
			_Callable(
				t,
				WORKER,
			),
		)
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	<-handle.Done()

	return handle
}

// TestHandle_ActsAsAStarlarkValue proves what a script sees of a handle. Each
// row is one rule of the value protocol:
//   - the type satisfies starlark.Value, at compile time rather than by hoping,
//     and names itself HANDLE_TYPE;
//   - printing one names the function and the thread id a workflow schema
//     records;
//   - `if h:` tests presence rather than outcome, since a handle that reported
//     its result through Truth would make a script branch on whether a thread
//     had finished, which is join's job;
//   - two spawns of one function cannot silently collapse into one entry of a
//     set or one key of a dict, so a handle refuses to hash;
//   - freezing is not silently dropping a mutation, since a handle exposes
//     nothing a script can mutate.
//
// Revisions:
//   - 2026-09-19 21:52: initial creation
//   - 2026-09-19 21:52: initial creation
//   - 2026-09-19 21:53: initial creation
//   - 2026-09-19 21:53: initial creation
//   - 2026-09-19 21:54: initial creation
//   - 2026-09-21 00:59: a thread id is a string that names its parent
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 22:45: merged TestHandle_StringNamesTheFunctionAndTheThread,
//     TestHandle_IsAlwaysTrue, TestHandle_RefusesToHash and
//     TestHandle_FreezeChangesNothing as rows, and moved to the external package,
//     with a handle from a real spawn (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHandle_ActsAsAStarlarkValue(t *testing.T) {
	tests := []struct {
		name  string
		check func(
			t *testing.T,
			handle common.Handle,
		)
	}{
		{
			name: "type",
			check: func(
				t *testing.T,
				handle common.Handle,
			) {
				t.Helper()

				var value starlark.Value = handle

				if value.Type() != run.HANDLE_TYPE {
					t.Fatalf(
						"type is %q, want %q",
						value.Type(),
						run.HANDLE_TYPE,
					)
				}
			},
		},
		{
			name: "string names the function and the thread",
			check: func(
				t *testing.T,
				handle common.Handle,
			) {
				t.Helper()

				got := handle.String()

				for _, want := range []string{
					run.HANDLE_TYPE,
					WORKER,
					"#" + WORKER_THREAD,
				} {
					if !strings.Contains(
						got,
						want,
					) {
						t.Fatalf(
							"String is %q, which does not carry %q",
							got,
							want,
						)
					}
				}
			},
		},
		{
			name: "always true",
			check: func(
				t *testing.T,
				handle common.Handle,
			) {
				t.Helper()

				if handle.Truth() != starlark.True {
					t.Fatal("a handle is falsy")
				}
			},
		},
		{
			name: "refuses to hash",
			check: func(
				t *testing.T,
				handle common.Handle,
			) {
				t.Helper()

				_, err := handle.Hash()
				if err == nil {
					t.Fatal("a handle hashed, so two spawns could collide")
				}

				if !strings.Contains(
					err.Error(),
					UNHASHABLE,
				) {
					t.Fatalf(
						"refusal does not say why: %v",
						err,
					)
				}
			},
		},
		{
			name: "freeze changes nothing",
			check: func(
				t *testing.T,
				handle common.Handle,
			) {
				t.Helper()

				before := handle.String()

				handle.Freeze()

				var (
					same  = handle.String() == before
					named = handle.Name() == WORKER
					lane  = handle.Thread() == WORKER_THREAD
				)

				if !same || !named || !lane {
					t.Fatal("freezing a handle changed it")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				tt.check(
					t,
					_Spawned(t),
				)
			},
		)
	}
}

// TestGuard_C112_HandleZeroReads proves a handle nothing was stored in reads as
// empty: no name, no thread, and a Done that is closed, as a zero handle was
// before the fields were guarded.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-09 02:01: moved to the external package, which reads a zero
//     Handle through its exported methods (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C112_HandleZeroReads(t *testing.T) {
	handle := new(run.Handle)

	if handle.Name() != "" || handle.Thread() != "" {
		t.Fatalf(
			"a zero handle has name %q and thread %q",
			handle.Name(),
			handle.Thread(),
		)
	}

	select {
	case <-handle.Done():
	default:
		t.Fatal("the Done of a zero handle is open")
	}

	_, done, _ := handle.Result()
	if !done {
		t.Fatal("a zero handle is not over")
	}
}
