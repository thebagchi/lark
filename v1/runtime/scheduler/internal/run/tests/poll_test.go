package tests

import (
	"context"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

const (
	// POLL_SOURCE holds the loops a cancelled thread must leave. Each calls
	// mark first, which cancels the context, and then loops for far longer
	// than the bound the test allows.
	//
	// Revisions:
	//   - 2026-11-02 11:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:45: moved to the external package (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	POLL_SOURCE = `
def pass_loop():
    mark()
    for _ in range(100000000):
        pass

def builtin_loop():
    mark()
    for _ in range(100000000):
        len("abc")

def then_effect():
    cancel_now()
    effect()
`
)

// _Polled compiles POLL_SOURCE against predeclared, with a builtin that does
// nothing for each name it lacks, and returns the thread of a run that
// run.Begin set up on ctx, which arms the watch on the run's context, and the
// global named name.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 22:45: moved to the external package; run.Begin arms the watch
//     that _Polled armed itself (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Polled(
	ctx context.Context,
	t *testing.T,
	predeclared starlark.StringDict,
	name string,
) (*starlark.Thread, starlark.Callable) {
	t.Helper()

	for _, absent := range []string{"mark", "cancel_now", "effect"} {
		_, found := predeclared[absent]
		if !found {
			predeclared[absent] = _Builtin(
				absent,
				func() {
					// Nothing: the script never reaches it.
				},
			)
		}
	}

	globals, err := starlark.ExecFileOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT_NAME,
		},
		SCRIPT_NAME,
		[]byte(POLL_SOURCE),
		predeclared,
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	target, ok := globals[name].(starlark.Callable)
	if !ok {
		t.Fatalf(
			"%s is not callable",
			name,
		)
	}

	thread, _ := _Open(
		ctx,
		t,
		nil,
	)

	return thread, target
}

// _Builtin is a builtin of fn that returns None.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Builtin(
	name string,
	fn func(),
) *starlark.Builtin {
	return starlark.NewBuiltin(
		name,
		func(
			_ *starlark.Thread,
			_ *starlark.Builtin,
			_ starlark.Tuple,
			_ []starlark.Tuple,
		) (starlark.Value, error) {
			fn()

			return starlark.None, nil
		},
	)
}

// TestCancelOn_StopsWithinPoll proves a thread whose context is done runs at
// most 2*POLL more steps. The signal is a count of steps, read off the
// thread, so the test races no clock: mark cancels the context and notes the
// step it did so at.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCancelOn_StopsWithinPoll(t *testing.T) {
	cases := []struct {
		name string
		loop string
	}{
		{
			"pass loop",
			"pass_loop",
		},
		{
			"builtin loop",
			"builtin_loop",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					ctx, stop = context.WithCancel(t.Context())
					marked    uint64
					thread    *starlark.Thread
					target    starlark.Callable
					mark      = _Builtin(
						"mark",
						func() {
							marked = thread.ExecutionSteps()

							stop()
						},
					)
				)

				defer stop()

				thread, target = _Polled(
					ctx,
					t,
					starlark.StringDict{
						"mark": mark,
					},
					item.loop,
				)

				_, err := starlark.Call(
					thread,
					target,
					nil,
					nil,
				)
				if err == nil || !strings.Contains(
					err.Error(),
					run.STOPPED,
				) {
					t.Fatalf(
						"got %v, want the run to be stopped",
						err,
					)
				}

				after := thread.ExecutionSteps() - marked
				if after > 2*common.POLL {
					t.Fatalf(
						"ran %d steps after the cancel, want at most %d",
						after,
						2*common.POLL,
					)
				}
			},
		)
	}
}

// TestPolled_StartsNoBuiltinAfterTheCancelIsSeen proves a builtin whose
// return finds the context done is the last builtin to run: the next one is
// never called. The control, an environment not wrapped, shows the test can
// fail: there the second builtin runs, the poll of the steps being far off.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestPolled_StartsNoBuiltinAfterTheCancelIsSeen(t *testing.T) {
	cases := []struct {
		name    string
		wrapped bool
		effects int
	}{
		{
			"wrapped",
			true,
			0,
		},
		{
			"control, not wrapped",
			false,
			1,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					ctx, stop = context.WithCancel(t.Context())
					effects   int
					env       = starlark.StringDict{
						"cancel_now": _Builtin(
							"cancel_now",
							func() {
								stop()
							},
						),
						"effect": _Builtin(
							"effect",
							func() {
								effects++
							},
						),
					}
					thread *starlark.Thread
					target starlark.Callable
				)

				defer stop()

				if item.wrapped {
					env = run.Polled(env)
				}

				thread, target = _Polled(
					ctx,
					t,
					env,
					"then_effect",
				)

				_, err := starlark.Call(
					thread,
					target,
					nil,
					nil,
				)
				if item.wrapped && (err == nil || !strings.Contains(
					err.Error(),
					run.STOPPED,
				)) {
					t.Fatalf(
						"got %v, want the run to be stopped",
						err,
					)
				}

				if effects != item.effects {
					t.Fatalf(
						"effect ran %d times, want %d",
						effects,
						item.effects,
					)
				}
			},
		)
	}
}

// TestPoll_LeavesABareThreadAlone proves Poll cancels nothing when the thread
// is bare, with no run and no hook. A thread with no hook would meet the
// interpreter's default, which cancels it with "too many steps", so Poll must
// not touch its step limit.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation, as TestPoll_IgnoresAThreadWithNoRun
//   - 2026-11-08 23:21: moved to the external package, out of the table of
//     TestPoll_LeavesAThreadWithNoHookAlone (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestPoll_LeavesABareThreadAlone(t *testing.T) {
	thread := &starlark.Thread{}

	run.Poll(thread)

	_, err := starlark.Call(
		thread,
		_Callable(
			t,
			WORKER,
		),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"the thread was cancelled: %v",
			err,
		)
	}
}

// TestPoll_LeavesAThreadWithNoHookAlone proves Poll cancels nothing when the
// thread carries a run that nothing watches. A thread with no hook would meet
// the interpreter's default, which cancels it with "too many steps", so Poll
// must not touch its step limit. The run ends first: that releases the watch
// and leaves the run on the thread. The bare thread, the other case, is
// TestPoll_LeavesABareThreadAlone.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation, as TestPoll_IgnoresAThreadWithNoRun
//   - 2026-11-08 23:32: moved to the external package; the watch is released by
//     ending a run that run.Begin started (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestPoll_LeavesAThreadWithNoHookAlone(t *testing.T) {
	thread, end := _Open(
		t.Context(),
		t,
		nil,
	)

	end()

	run.Poll(thread)

	_, err := starlark.Call(
		thread,
		_Callable(
			t,
			WORKER,
		),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"the thread was cancelled: %v",
			err,
		)
	}
}
