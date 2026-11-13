package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestNilLive proves a nil or zero argument does not panic when the thread
// carries a run, which is the case TestNilSweepHelpers cannot reach: a thread
// with no run is refused with scheduler.ErrRun before the argument is read.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 fix round 3)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: updated for the move of Handle, Evaluation and Hold to common (C91w5)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestNilLive(t *testing.T) {
	var (
		noBuiltin *starlark.Builtin
		builtin   = starlark.NewBuiltin(
			"f",
			func(
				*starlark.Thread,
				*starlark.Builtin,
				starlark.Tuple,
				[]starlark.Tuple,
			) (starlark.Value, error) {
				return starlark.None, nil
			},
		)
		live = func(t *testing.T) *starlark.Thread {
			t.Helper()

			var (
				thread = new(starlark.Thread)
				finish = scheduler.Begin(
					context.Background(),
					thread,
					"x",
					&common.Settings{},
				)
			)

			t.Cleanup(func() {
				_ = finish(nil)
			})

			return thread
		}
	)

	cases := []struct {
		name string
		call func(thread *starlark.Thread) error
		want error
	}{
		{
			"Wait nil handle",
			func(thread *starlark.Thread) error {
				return _Last(scheduler.Wait(
					thread,
					nil,
				))
			},
			nil,
		},
		{
			"Settle nil handle",
			func(thread *starlark.Thread) error {
				scheduler.Settle(
					thread,
					nil,
				)

				return nil
			},
			nil,
		},
		{
			"Fork nil target",
			func(thread *starlark.Thread) error {
				return _Last(scheduler.Fork(
					thread,
					nil,
				))
			},
			scheduler.ErrRun,
		},
		{
			"Fork typed nil target",
			func(thread *starlark.Thread) error {
				return _Last(scheduler.Fork(
					thread,
					noBuiltin,
				))
			},
			scheduler.ErrRun,
		},
		{
			"Fork nil option",
			func(thread *starlark.Thread) error {
				handle, err := scheduler.Fork(
					thread,
					builtin,
					nil,
				)
				if err == nil {
					_, err = scheduler.Wait(
						thread,
						handle,
					)
				}

				return err
			},
			nil,
		},
		{
			"Within nil target",
			func(thread *starlark.Thread) error {
				return _Last(scheduler.Within(
					thread,
					nil,
				))
			},
			scheduler.ErrRun,
		},
		{
			"Within typed nil target",
			func(thread *starlark.Thread) error {
				return _Last(scheduler.Within(
					thread,
					noBuiltin,
				))
			},
			scheduler.ErrRun,
		},
		{
			"Within nil option",
			func(thread *starlark.Thread) error {
				return _Last(scheduler.Within(
					thread,
					builtin,
					nil,
				))
			},
			nil,
		},
		{
			"SharedIn nil build for a key not held",
			func(thread *starlark.Thread) error {
				evaluation, err := scheduler.EvaluationOf(thread)
				if err != nil {
					return err
				}

				return _Last(scheduler.SharedIn[int](
					evaluation,
					"absent",
					nil,
				))
			},
			scheduler.ErrRun,
		},
		{
			"SharedIn nil build for a key held",
			func(thread *starlark.Thread) error {
				evaluation, err := scheduler.EvaluationOf(thread)
				if err != nil {
					return err
				}

				_, err = scheduler.SharedIn[int](
					evaluation,
					"held",
					func() int {
						return 1
					},
				)
				if err != nil {
					return err
				}

				return _Last(scheduler.SharedIn[int](
					evaluation,
					"held",
					nil,
				))
			},
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					thread = live(t)
					err    error
				)

				_Survives(
					t,
					func() {
						err = tc.call(thread)
					},
				)

				if !errors.Is(
					err,
					tc.want,
				) && (tc.want != nil || err != nil) {
					t.Fatalf(
						"want %v, got %v",
						tc.want,
						err,
					)
				}
			},
		)
	}
}

// TestNilCheckPaddingGroup proves CheckPadding refuses padding it cannot
// measure when the group is zero or negative, and does not divide by zero.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 fix round 3)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestNilCheckPaddingGroup(t *testing.T) {
	for _, group := range []int{0, -1} {
		_Survives(
			t,
			func() {
				_, ok := unpack.CheckPadding(
					"a==",
					group,
				)
				if ok {
					t.Fatalf(
						"group %d: want false, got true",
						group,
					)
				}
			},
		)
	}
}

// TestNilZeroOwners proves a zero Walk and a nil read function reach nothing,
// without a panic.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 fix round 3)
//   - 2026-11-07 19:54: the Listener cases moved to remote/internal/host (C91w8c)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrSource, formerly ErrNoSource (C121, Q348)
func TestNilZeroOwners(t *testing.T) {
	var (
		dir    = t.TempDir()
		module = filepath.Join(
			dir,
			"m.star",
		)
		entry = filepath.Join(
			dir,
			"entry.star",
		)
	)

	err := os.WriteFile(
		module,
		[]byte("x = 1\n"),
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		call func() error
		want error
	}{
		{
			"zero Walk Reach",
			func() error {
				_, err := new(script.Walk).Reach(
					"",
					"x",
					nil,
				)

				return err
			},
			script.ErrSource,
		},
		{
			"Walk Reach with a nil read function",
			func() error {
				_, err := script.NewWalk(
					nil,
					entry,
				).Reach(
					entry,
					"m.star",
					nil,
				)

				return err
			},
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var err error

				_Survives(
					t,
					func() {
						err = tc.call()
					},
				)

				if !errors.Is(
					err,
					tc.want,
				) && (tc.want != nil || err != nil) {
					t.Fatalf(
						"want %v, got %v",
						tc.want,
						err,
					)
				}
			},
		)
	}
}

// TestNilStopError proves a nil StopError answers Error with a fixed text and
// Unwrap with nil, without a panic.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 fix round 4)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestNilStopError(t *testing.T) {
	var stop *script.StopError

	cases := []struct {
		name string
		call func() bool
	}{
		{
			"Error",
			func() bool {
				return stop.Error() != ""
			},
		},
		{
			"Unwrap",
			func() bool {
				return stop.Unwrap() == nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var ok bool

				_Survives(
					t,
					func() {
						ok = tc.call()
					},
				)

				if !ok {
					t.Fatal("wrong answer for a nil StopError")
				}
			},
		)
	}
}

// TestNilRegister proves Register adds nothing for a nil plugin, so Registered
// still builds an environment afterwards.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 rebase onto C93)
//   - 2026-11-07 19:00: comment says Registered, not DEFAULT (C91w1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestNilRegister(t *testing.T) {
	var (
		before = len(plugin.Registered())
		err    error
	)

	_Survives(
		t,
		func() {
			plugin.Register(nil)

			_, err = plugin.Environment(plugin.Registered())
		},
	)

	if len(plugin.Registered()) != before {
		t.Fatalf(
			"want %d registered plugins, got %d",
			before,
			len(plugin.Registered()),
		)
	}

	if err != nil {
		t.Fatal(err)
	}
}
