package tests

// This file tests Reach, NotNested, Joined, Recorded, Assert, Pause and Global
// against a nil receiver, a nil builtin, a nil handle and a nil map. No script
// can build these inputs, so the tests call the functions directly.
//
// Revisions:
//   - 2026-11-03 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from guard_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// _Script runs source and returns the function named name it defines.
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Script(
	t *testing.T,
	source string,
	name string,
) *starlark.Function {
	t.Helper()

	globals, err := starlark.ExecFileOptions(
		new(syntax.FileOptions),
		new(starlark.Thread),
		"guard.star",
		source,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"script: %v",
			err,
		)
	}

	fn, ok := globals[name].(*starlark.Function)
	if !ok {
		t.Fatalf(
			"%s is not a function",
			name,
		)
	}

	return fn
}

// TestGuard_ReachNilReceiver proves every walk method on a nil receiver
// answers scheduler.ErrRun and does not panic.
//
// Cases from Go-level breaker run 3, findings scheduler-F7 and F9
// (bugs B062, B064).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_ReachNilReceiver(t *testing.T) {
	var (
		reach *builtin.Reach
		fn    = _Script(
			t,
			"def f(): pass",
			"f",
		)
	)

	tests := []struct {
		name string
		call func() error
	}{
		{
			"_Value",
			func() error {
				return reach.Value(starlark.String("x"))
			},
		},
		{
			"_Value of a tuple",
			func() error {
				return reach.Value(starlark.Tuple{starlark.String("a")})
			},
		},
		{
			"_Value of a dict",
			func() error {
				return reach.Value(starlark.NewDict(0))
			},
		},
		{
			"_Container",
			func() error {
				return reach.Container(starlark.NewList(nil))
			},
		},
		{
			"_Function",
			func() error {
				return reach.Function(fn)
			},
		},
		{
			"_Iterable",
			func() error {
				return reach.Iterable(starlark.NewList(nil))
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
						"got %v, want scheduler.ErrRun",
						err,
					)
				}
			},
		)
	}

	if reach.First(starlark.String("x")) {
		t.Fatal("a nil receiver met a value for the first time")
	}
}

// TestGuard_ReachZeroValue proves a Reach built with new, which has no visit,
// no seen map and no path, walks every kind of value without a panic. A walk
// with no visit refuses a function with scheduler.ErrRun.
//
// Cases from Go-level breaker run 3, findings scheduler-F5, F7, F9 and F14
// (bugs B060, B062, B064, B069).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_ReachZeroValue(t *testing.T) {
	fn := _Script(
		t,
		"def f(): pass",
		"f",
	)

	t.Run(
		"function with no visit",
		func(t *testing.T) {
			err := new(builtin.Reach).Function(fn)
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
		"first with no seen map",
		func(t *testing.T) {
			reach := new(builtin.Reach)

			if !reach.First(starlark.String("x")) {
				t.Fatal("first meeting was not first")
			}

			if reach.First(starlark.String("x")) {
				t.Fatal("second meeting was first")
			}
		},
	)

	t.Run(
		"nil iterable",
		func(t *testing.T) {
			err := new(builtin.Reach).Iterable(nil)
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
		"nil function, dict and method",
		func(t *testing.T) {
			reach := (*builtin.Descent)(new(builtin.Reach))

			for _, err := range []error{
				reach.Function(nil),
				reach.Dict(nil),
				reach.Receiver(nil),
			} {
				if !errors.Is(
					err,
					scheduler.ErrRun,
				) {
					t.Fatalf(
						"got %v, want scheduler.ErrRun",
						err,
					)
				}
			}
		},
	)

	values := []starlark.Value{
		starlark.String("a"),
		starlark.Tuple{starlark.String("a")},
		starlark.NewList([]starlark.Value{starlark.String("a")}),
		starlark.NewDict(0),
		starlark.NewSet(0),
	}

	for _, value := range values {
		t.Run(
			value.Type(),
			func(t *testing.T) {
				err := new(builtin.Reach).Value(value)
				if err != nil {
					t.Fatalf(
						"got %v, want nil",
						err,
					)
				}
			},
		)
	}
}

// TestGuard_ReachShared proves ten goroutines can share one Reach without a
// data race. The -race flag is what fails this test on the unguarded code.
//
// Case from Go-level breaker run 3, finding scheduler-F8 (bug B063). Lark
// never shares a Reach (see _Walk), so this guards a misuse and not a use.
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_ReachShared(t *testing.T) {
	var (
		group sync.WaitGroup
		inner = starlark.NewList([]starlark.Value{
			starlark.String("a"),
			starlark.MakeInt(1),
		})
		reach = &builtin.Reach{
			Seen: make(map[starlark.Value]bool),
			Path: trail.Bounded(context.Background()),
		}
	)

	for range 10 {
		group.Add(1)

		go func() {
			defer group.Done()

			_ = reach.Container(inner)
			_ = reach.Value(starlark.String("x"))
		}()
	}

	group.Wait()
}

// TestGuard_NotNested proves a nil builtin, a nil thread or both are an error
// and not a panic.
//
// Cases from Go-level breaker run 3, findings scheduler-F10 and F13
// (bugs B065, B068).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_NotNested(t *testing.T) {
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
				"join",
				nil,
			),
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := builtin.NotNested(
					nil,
					test.fn,
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

// TestGuard_Joined proves a nil thread is scheduler.ErrRun, and a nil wait on a thread
// that carries a run is "nothing to call". Neither is a panic.
//
// Case from Go-level breaker run 3, finding scheduler-F13 (bug B068).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_Joined(t *testing.T) {
	var wait builtin.Wait

	t.Run(
		"nil thread",
		func(t *testing.T) {
			_, err := builtin.Joined(
				nil,
				nil,
				nil,
				wait,
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

	t.Run(
		"nil wait",
		func(t *testing.T) {
			var (
				thread = new(starlark.Thread)
				finish = scheduler.Begin(
					context.Background(),
					thread,
					"guard",
					nil,
				)
				_, err = builtin.Joined(
					thread,
					nil,
					nil,
					wait,
				)
				produced = finish(nil)
			)
			if produced != nil {
				t.Fatalf(
					"the run produced %v",
					produced,
				)
			}

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

// TestGuard_Recorded proves a nil handle among the handles is ErrHandle
// and not a panic.
//
// Case from Go-level breaker run 3, finding scheduler-F11 (bug B066).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-07 20:07: uses common.Handle (C91w5, Q127)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrHandle, formerly ErrNotAHandle (C121, Q348)
func TestGuard_Recorded(t *testing.T) {
	_, err := builtin.Recorded(
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
}

// TestGuard_Assert proves a nil thread does not panic in either failing shape
// of assert, and that the cause comes back as it would for a thread with no
// run.
//
// Case from Go-level breaker run 3, finding scheduler-F12 (bug B067).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrCondition, formerly ErrNotACondition (C121, Q348)
func TestGuard_Assert(t *testing.T) {
	tests := []struct {
		name string
		args starlark.Tuple
		want error
	}{
		{
			"false condition",
			starlark.Tuple{starlark.False},
			core.ErrAssert,
		},
		{
			"bare string",
			starlark.Tuple{starlark.String("text")},
			core.ErrCondition,
		},
		{
			"no condition",
			starlark.Tuple{},
			core.ErrAssert,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := builtin.Assert(
					nil,
					nil,
					test.args,
					nil,
				)
				if !errors.Is(
					err,
					test.want,
				) {
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

// TestGuard_Pause proves a nil builtin does not panic when sleep reads its
// arguments, whether they are good or bad.
//
// Case from Go-level breaker run 3, finding scheduler-F15 (bug B070).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_Pause(t *testing.T) {
	tests := []struct {
		name    string
		args    starlark.Tuple
		wantErr bool
	}{
		{
			"no arguments",
			nil,
			true,
		},
		{
			"a duration",
			starlark.Tuple{starlark.MakeInt(5)},
			false,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := builtin.Pause(
					nil,
					test.args,
					nil,
				)
				if (err != nil) != test.wantErr {
					t.Fatalf(
						"got %v, want error %v",
						err,
						test.wantErr,
					)
				}
			},
		)
	}
}

// TestGuard_Global proves a nil function is the global of nothing.
//
// Case from Go-level breaker run 3, finding scheduler-F16 (bug B071).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_Global(t *testing.T) {
	if builtin.Global(nil) {
		t.Fatal("a nil function was reported as a global")
	}
}
