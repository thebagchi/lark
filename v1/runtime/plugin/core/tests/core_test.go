package tests

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	SCRIPT_NAME   = "core_test.star"
	ENTRY         = "main"
	WORKER        = "worker"
	SECOND        = "second"
	FAILING       = "failing"
	SPINNER       = "spin"
	WAITER        = "waits"
	TAKES_ARGS    = "wants"
	NOT_A_FUNC    = "NUMBER"
	ANON_FUNC     = "ANON"
	ISOLATED      = "isolated"
	DEFAULTED     = "defaulted"
	CAPTURES      = "captures"
	THROUGH       = "through"
	HANDING       = "handing"
	FROZEN        = "frozen"
	PAIR          = 2
	FAILURE_TEXT  = "a spawned failure"
	WHY           = "two and two"
	EXPECTED_NAME = "core"
	STOP_AFTER    = 20 * time.Millisecond
	LONG_SLEEP    = 30000
	LIMIT         = 5 * time.Second
	SETTLE_TRIES  = 50
	SETTLE_WAIT   = 10 * time.Millisecond
)

// SUPPLIED is every name this plugin puts in a script's environment, written
// once, so the test and the plugin cannot disagree about the count.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	SUPPLIED = []string{
		common.SPAWN,
		common.JOIN,
		spec.OUTCOMES,
		common.CANCEL,
		spec.ASSERT,
		common.SLEEP,
		spec.STR,
		spec.REPR,
		spec.PRINT,
		spec.FAIL,
	}
)

const (
	SOURCE = `
NUMBER = 1
ANON = lambda: 1

def worker():
    return 7

def second():
    return 9

def failing():
    fail("a spawned failure")

def wants(x):
    return x

def spin():
    total = 0
    for i in range(100000000):
        total += i
    return total

def waits():
    return join(spawn(spin))

def isolated():
    assert(False, "two and two")
    return 1

def defaulted(x = 5):
    return x * 2

def captures():
    x = 1

    def reads():
        return x

    return reads

def through():
    x = 1

    def reads():
        return x

    def outer(fn = reads):
        return fn()

    return outer

def handing():
    items = []

    return (items, lambda items = items: len(items))
`
)

// _Globals compiles SOURCE against the plugin's names and freezes what it
// produced.
//
// Revisions:
//   - 2026-09-19 22:03: initial creation
//   - 2026-09-21 09:46: through the plugin's own surface
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Globals(t *testing.T) starlark.StringDict {
	t.Helper()

	globals, err := starlark.ExecFileOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT_NAME,
		},
		SCRIPT_NAME,
		[]byte(SOURCE),
		core.Builtins(),
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	globals.Freeze()

	return globals
}

// _Begun returns a thread carrying a fresh run, as Eval would leave it
// around a call, and ends the run when the test does, failing the test if the
// run produced an error.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-10-02 12:19: the function that ends the run takes the evaluation's
//     error, as Begin's does
//   - 2026-10-03 20:54: ends the run itself, and checks what it produced, which Begin's
//     function now answers with
//   - 2026-10-24 18:29: takes the context first
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
func _Begun(
	ctx context.Context,
	t *testing.T,
) *starlark.Thread {
	t.Helper()

	var (
		thread = &starlark.Thread{
			Name: ENTRY,
		}
		finish = scheduler.Begin(
			ctx,
			thread,
			ENTRY,
			nil,
		)
	)

	t.Cleanup(func() {
		produced := finish(nil)
		if produced != nil {
			t.Errorf(
				"the run produced %v",
				produced,
			)
		}
	})

	return thread
}

// _Call invokes one of the plugin's builtins on thread with the values given.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Call(
	t *testing.T,
	thread *starlark.Thread,
	name string,
	args ...starlark.Value,
) (starlark.Value, error) {
	t.Helper()

	return starlark.Call(
		thread,
		core.Builtins()[name],
		starlark.Tuple(args),
		nil,
	)
}

// _Spawned spawns the named global and returns the handle.
//
// Revisions:
//   - 2026-09-19 22:04: initial creation
//   - 2026-11-07 20:07: uses common.Handle (C91w5, Q127)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Spawned(
	t *testing.T,
	thread *starlark.Thread,
	name string,
) common.Handle {
	t.Helper()

	value, err := _Call(
		t,
		thread,
		common.SPAWN,
		_Globals(t)[name],
	)
	if err != nil {
		t.Fatalf(
			"spawn %s: %v",
			name,
			err,
		)
	}

	handle, ok := value.(common.Handle)
	if !ok {
		t.Fatalf(
			"spawn returned %T, want common.Handle",
			value,
		)
	}

	return handle
}

// TestBuiltins_SuppliesItsNames proves what a host gets, and that each call
// builds a fresh map rather than handing out one shared with every other host.
//
// Revisions:
//   - 2026-09-19 22:23: initial creation
//   - 2026-09-21 09:46: on the plugin
//   - 2026-11-02 16:28: expects outcomes (C28)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBuiltins_SuppliesItsNames(t *testing.T) {
	values := core.Builtins()

	if len(values) != len(SUPPLIED) {
		t.Fatalf(
			"supplies %v, want %v",
			values.Keys(),
			SUPPLIED,
		)
	}

	for _, name := range SUPPLIED {
		builtin, ok := values[name].(*starlark.Builtin)
		if !ok {
			t.Fatalf(
				"%s is %T, want *starlark.Builtin",
				name,
				values[name],
			)
		}

		if builtin.Name() != name {
			t.Fatalf(
				"%s is named %q",
				name,
				builtin.Name(),
			)
		}
	}

	values[common.SPAWN] = starlark.None

	if core.Builtins()[common.SPAWN] == starlark.None {
		t.Fatal("changing one host's environment changed everybody's")
	}
}

// TestSpawn_RunsAndJoins proves the whole shape through the surface: a spawn
// returns a handle, and a join returns what it produced, in argument order.
//
// Revisions:
//   - 2026-09-19 22:11: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpawn_RunsAndJoins(t *testing.T) {
	thread := _Begun(
		t.Context(),
		t,
	)

	var (
		first = _Spawned(
			t,
			thread,
			WORKER,
		)
		second = _Spawned(
			t,
			thread,
			SECOND,
		)
		value, err = _Call(
			t,
			thread,
			common.JOIN,
			first,
			second,
		)
	)
	if err != nil {
		t.Fatalf(
			"join: %v",
			err,
		)
	}

	if value.String() != "[7, 9]" {
		t.Fatalf(
			"join returned %s, want [7, 9]",
			value,
		)
	}

	if first.Thread() != "thread_1" || second.Thread() != "thread_2" {
		t.Fatalf(
			"lanes are %s and %s",
			first.Thread(),
			second.Thread(),
		)
	}
}

// TestSpawn_RefusesWhatCannotBeCalled proves each refusal, and that all reach
// one sentinel a host can act on.
//
// Revisions:
//   - 2026-09-19 22:06: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrFunction, formerly ErrNotAName (C121, Q348)
func TestSpawn_RefusesWhatCannotBeCalled(t *testing.T) {
	var (
		globals = _Globals(t)
		thread  = _Begun(
			t.Context(),
			t,
		)
		cases = []struct {
			name string
			args []starlark.Value
		}{
			{
				name: "nothing",
				args: nil,
			},
			{
				name: "two functions",
				args: []starlark.Value{globals[WORKER], globals[WORKER]},
			},
			{
				name: "not a function",
				args: []starlark.Value{globals[NOT_A_FUNC]},
			},
			{
				name: "takes arguments",
				args: []starlark.Value{globals[TAKES_ARGS]},
			},
		}
	)

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Call(
					t,
					thread,
					common.SPAWN,
					item.args...,
				)
				if !errors.Is(
					err,
					core.ErrFunction,
				) {
					t.Fatalf(
						"got %v, want ErrFunction",
						err,
					)
				}
			},
		)
	}
}

// TestSpawn_TakesALambda records that a compiler's spawn(lambda: ...) is
// accepted, and has no name when nothing says what it calls.
//
// Revisions:
//   - 2026-09-20 20:53: initial creation
//   - 2026-10-02 00:48: has no name rather than the interpreter's word for a
//     lambda, which would read like a function of that name
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpawn_TakesALambda(t *testing.T) {
	thread := _Begun(
		t.Context(),
		t,
	)

	if got := _Spawned(
		t,
		thread,
		ANON_FUNC,
	).Name(); got != "" {
		t.Fatalf(
			"want no name, got %q",
			got,
		)
	}
}

// TestSpawn_TakesTheLabelTheDialectPasses checks the hidden keywords the
// dialect writes on a spawn: the function a lambda calls names the thread, and
// neither keyword reaches the arguments spawn reads.
//
// Revisions:
//   - 2026-10-02 00:48: initial creation
//   - 2026-11-07 20:07: uses common.Handle (C91w5, Q127)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpawn_TakesTheLabelTheDialectPasses(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		kwargs = []starlark.Tuple{
			{starlark.String(common.BINDING), starlark.String("h1")},
			{starlark.String(common.CALLEE), starlark.String(WORKER)},
		}
		args       = starlark.Tuple{_Globals(t)[ANON_FUNC]}
		value, err = starlark.Call(
			thread,
			core.Builtins()[common.SPAWN],
			args,
			kwargs,
		)
	)
	if err != nil {
		t.Fatalf(
			"spawn: %v",
			err,
		)
	}

	handle, ok := value.(common.Handle)
	if !ok || handle.Name() != WORKER {
		t.Fatalf(
			"got %v, want a handle named %s",
			value,
			WORKER,
		)
	}
}

// TestSpawn_TakesAFunctionWhoseParametersAllHaveDefaults proves a function
// spawn need supply nothing to is accepted, which is what a lambda written
// inside spawn is once the dialect has compiled it.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpawn_TakesAFunctionWhoseParametersAllHaveDefaults(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		handle = _Spawned(
			t,
			thread,
			DEFAULTED,
		)
		value, err = _Call(
			t,
			thread,
			common.JOIN,
			handle,
		)
	)
	if err != nil {
		t.Fatalf(
			"join: %v",
			err,
		)
	}

	if value.String() != "[10]" {
		t.Fatalf(
			"join returned %s, want [10]",
			value,
		)
	}
}

// TestSpawn_RefusesWhatCaptures proves a function reaching the new thread that
// captures a local variable is refused, whether it is the one spawned or one
// that function's default holds.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrClosure, formerly ErrCaptures (C121, Q348)
func TestSpawn_RefusesWhatCaptures(t *testing.T) {
	thread := _Begun(
		t.Context(),
		t,
	)

	for _, name := range []string{CAPTURES, THROUGH} {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := _Call(
					t,
					thread,
					common.SPAWN,
					_Made(
						t,
						thread,
						name,
					),
				)
				if !errors.Is(
					err,
					core.ErrClosure,
				) {
					t.Fatalf(
						"got %v, want ErrClosure",
						err,
					)
				}
			},
		)
	}
}

// TestSpawn_FreezesWhatTheThreadIsHanded proves a value a spawned function
// holds is frozen, so the thread that handed it over cannot change it under
// the thread it went to.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSpawn_FreezesWhatTheThreadIsHanded(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		made = _Made(
			t,
			thread,
			HANDING,
		)
		pair, ok = made.(starlark.Tuple)
	)
	if !ok || len(pair) != PAIR {
		t.Fatalf(
			"%s made %s, want a list and a lambda",
			HANDING,
			made,
		)
	}

	items, ok := pair[0].(*starlark.List)
	if !ok {
		t.Fatalf(
			"%s made %s first, want a list",
			HANDING,
			pair[0].Type(),
		)
	}

	_, err := _Call(
		t,
		thread,
		common.SPAWN,
		pair[1],
	)
	if err != nil {
		t.Fatalf(
			"spawn: %v",
			err,
		)
	}

	err = items.Append(starlark.MakeInt(1))
	if err == nil || !strings.Contains(
		err.Error(),
		FROZEN,
	) {
		t.Fatalf(
			"appending to what a thread was handed: %v, want it %s",
			err,
			FROZEN,
		)
	}
}

// _Made calls the global function name on thread and returns what it made.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Made(
	t *testing.T,
	thread *starlark.Thread,
	name string,
) starlark.Value {
	t.Helper()

	value, err := starlark.Call(
		thread,
		_Globals(t)[name],
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"%s: %v",
			name,
			err,
		)
	}

	return value
}

// TestSpawn_RefusesAThreadWithNoRun proves spawn called outside a run is told
// so, rather than reaching a nil run.
//
// Revisions:
//   - 2026-09-19 22:07: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestSpawn_RefusesAThreadWithNoRun(t *testing.T) {
	_, err := _Call(
		t,
		&starlark.Thread{
			Name: SCRIPT_NAME,
		},
		common.SPAWN,
		_Globals(t)[WORKER],
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
}

// TestJoin_ReraisesAFailure proves a caller cannot take a result list and be
// unaware that one thread never produced anything.
//
// Revisions:
//   - 2026-09-19 22:12: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestJoin_ReraisesAFailure(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		_, err = _Call(
			t,
			thread,
			common.JOIN,
			_Spawned(
				t,
				thread,
				FAILING,
			),
		)
	)
	if err == nil {
		t.Fatal("joining a failed thread succeeded")
	}

	for _, want := range []string{FAILING, FAILURE_TEXT} {
		if !strings.Contains(
			err.Error(),
			want,
		) {
			t.Fatalf(
				"the failure does not carry %q: %v",
				want,
				err,
			)
		}
	}
}

// TestJoin_FirstFailureInArgumentOrderWins proves the outcome depends on how
// the script was written, not on which thread lost a race.
//
// Revisions:
//   - 2026-09-19 22:13: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestJoin_FirstFailureInArgumentOrderWins(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		slow = _Spawned(
			t,
			thread,
			SPINNER,
		)
		quick = _Spawned(
			t,
			thread,
			FAILING,
		)
	)

	slow.Stop()

	_, err := _Call(
		t,
		thread,
		common.JOIN,
		slow,
		quick,
	)
	if !errors.Is(
		err,
		scheduler.ErrCancelled,
	) {
		t.Fatalf(
			"got %v, want the first argument's failure (ErrCancelled)",
			err,
		)
	}
}

// TestJoin_AbandonsNothingWhenItGivesUp proves the half of fail-fast that is
// easy to leave out: when a handle fails, the ones join has not reached are
// cancelled and waited for, so no evaluation is still unwinding after join
// returns.
//
// Revisions:
//   - 2026-09-19 22:14: initial creation
//   - 2026-09-20 00:15: renamed and re-documented for what it actually proves
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestJoin_AbandonsNothingWhenItGivesUp(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		quick = _Spawned(
			t,
			thread,
			FAILING,
		)
		slow = _Spawned(
			t,
			thread,
			SPINNER,
		)
		joined = make(
			chan error,
			1,
		)
	)

	go func() {
		_, err := _Call(
			t,
			thread,
			common.JOIN,
			quick,
			slow,
		)
		joined <- err
	}()

	select {
	case err := <-joined:
		if err == nil {
			t.Fatal("a join with a failed handle succeeded")
		}
	case <-time.After(LIMIT):
		t.Fatal("join did not return")
	}

	select {
	case <-slow.Done():
	default:
		t.Fatal("join returned while a handle it was given was still running")
	}
}

// TestTimeout_ReachesAJoin is the rule a timeout rests on, seen from the
// builtins: stopping an evaluation that is joining a spawn it made ends in
// milliseconds, because the spawn descends from that evaluation.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestTimeout_ReachesAJoin(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		target, ok = _Globals(t)[WAITER].(starlark.Callable)
	)
	if !ok {
		t.Fatalf(
			"%s is not callable",
			WAITER,
		)
	}

	bounded, err := scheduler.Fork(
		thread,
		target,
		scheduler.Inline(),
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	time.Sleep(STOP_AFTER)
	bounded.Stop()

	select {
	case <-bounded.Done():
	case <-time.After(LIMIT):
		t.Fatal("stopping an evaluation did not stop the join inside it")
	}
}

// TestCancel_StopsWithoutWaiting proves cancel is not a join: it returns None
// and does not block, and a cancelled handle raises when joined.
//
// Revisions:
//   - 2026-09-19 22:15: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCancel_StopsWithoutWaiting(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		handle = _Spawned(
			t,
			thread,
			SPINNER,
		)
		value, err = _Call(
			t,
			thread,
			common.CANCEL,
			handle,
		)
	)
	if err != nil {
		t.Fatalf(
			"cancel: %v",
			err,
		)
	}

	if value != starlark.None {
		t.Fatalf(
			"cancel returned %v, want None",
			value,
		)
	}

	_, err = _Call(
		t,
		thread,
		common.JOIN,
		handle,
	)
	if !errors.Is(
		err,
		scheduler.ErrCancelled,
	) {
		t.Fatalf(
			"joining a cancelled handle gave %v, want ErrCancelled",
			err,
		)
	}
}

// TestCancel_IsIdempotent proves cancelling a handle twice, or one that has
// already finished, is not an error.
//
// Revisions:
//   - 2026-09-19 22:16: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCancel_IsIdempotent(t *testing.T) {
	var (
		thread = _Begun(
			t.Context(),
			t,
		)
		handle = _Spawned(
			t,
			thread,
			WORKER,
		)
	)

	if _, err := scheduler.Wait(
		thread,
		handle,
	); err != nil {
		t.Fatal(err)
	}

	for attempt := range 2 {
		_, err := _Call(
			t,
			thread,
			common.CANCEL,
			handle,
		)
		if err != nil {
			t.Fatalf(
				"cancel %d of a finished handle: %v",
				attempt,
				err,
			)
		}
	}
}

// TestHandles_RefusesWhatIsNotAHandle proves both builtins read their arguments
// through one guard, and that a keyword argument is refused too.
//
// Revisions:
//   - 2026-09-19 22:17: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrHandle, formerly ErrNotAHandle (C121, Q348)
func TestHandles_RefusesWhatIsNotAHandle(t *testing.T) {
	thread := _Begun(
		t.Context(),
		t,
	)

	for _, name := range []string{common.JOIN, common.CANCEL} {
		t.Run(
			name+" a number",
			func(t *testing.T) {
				_, err := _Call(
					t,
					thread,
					name,
					starlark.MakeInt(1),
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
			name+" a keyword",
			func(t *testing.T) {
				var (
					kwargs = []starlark.Tuple{
						{
							starlark.String("x"),
							starlark.MakeInt(1),
						},
					}
					_, err = starlark.Call(
						thread,
						core.Builtins()[name],
						nil,
						kwargs,
					)
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
	}
}

// _Settled polls until the goroutine count is back at or under before, or
// gives up.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Settled(before int) bool {
	for range SETTLE_TRIES {
		if runtime.NumGoroutine() <= before {
			return true
		}

		time.Sleep(SETTLE_WAIT)
	}

	return false
}

// TestRun_LeavesNoGoroutineBehind proves a cancel and a completion both reach
// all the way down: once the run has ended, every goroutine it started and
// every watcher those started are gone.
//
// Revisions:
//   - 2026-09-19 22:14: initial creation, as two tests on the scheduler
//   - 2026-09-21 09:46: through the surface, ending the run
//   - 2026-10-03 20:55: begins the run itself, since it ends it partway, and checks what
//     the run produced
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestRun_LeavesNoGoroutineBehind(t *testing.T) {
	var (
		before = runtime.NumGoroutine()
		thread = &starlark.Thread{
			Name: ENTRY,
		}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			ENTRY,
			nil,
		)
		spinner = _Spawned(
			t,
			thread,
			SPINNER,
		)
		worker = _Spawned(
			t,
			thread,
			WORKER,
		)
	)

	if _, err := _Call(
		t,
		thread,
		common.CANCEL,
		spinner,
	); err != nil {
		t.Fatalf(
			"cancel: %v",
			err,
		)
	}

	if _, err := _Call(
		t,
		thread,
		common.JOIN,
		worker,
	); err != nil {
		t.Fatalf(
			"join: %v",
			err,
		)
	}

	produced := finish(nil)
	if produced != nil {
		t.Fatalf(
			"the run produced %v",
			produced,
		)
	}

	if !_Settled(before) {
		t.Fatalf(
			"goroutines did not settle: %d before, %d after",
			before,
			runtime.NumGoroutine(),
		)
	}
}

// TestAssert_UsesStarlarkTruthiness proves the condition is judged the way the
// language's own if judges it, and that a true one is not an event.
//
// Outside a run, so a failure is raised and ends nothing; what a failure does
// to a run is the next test's.
//
// Revisions:
//   - 2026-09-19 22:21: initial creation
//   - 2026-09-20 00:01: a lone string is no longer a condition
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestAssert_UsesStarlarkTruthiness(t *testing.T) {
	bare := &starlark.Thread{
		Name: SCRIPT_NAME,
	}

	for _, value := range []starlark.Value{
		starlark.MakeInt(0),
		starlark.NewList(nil),
		starlark.None,
	} {
		_, err := _Call(
			t,
			bare,
			spec.ASSERT,
			value,
		)
		if !errors.Is(
			err,
			core.ErrAssert,
		) {
			t.Fatalf(
				"assert(%v) gave %v, want ErrAssert",
				value,
				err,
			)
		}
	}

	value, err := _Call(
		t,
		bare,
		spec.ASSERT,
		starlark.MakeInt(1),
		starlark.String(WHY),
	)
	if err != nil || value != starlark.None {
		t.Fatalf(
			"assert(1) gave %v, %v",
			value,
			err,
		)
	}

	_, err = _Call(
		t,
		bare,
		spec.ASSERT,
		starlark.False,
		starlark.String(WHY),
	)
	if !errors.Is(
		err,
		core.ErrAssert,
	) || !strings.Contains(
		err.Error(),
		WHY,
	) {
		t.Fatalf(
			"want ErrAssert carrying the message, got %v",
			err,
		)
	}
}

// TestAssert_RefusesALoneString proves the trap is closed: assert("text")
// reads like an unconditional failure and used to behave like a passing test.
// The refusal ends the run like any assertion, and the keyword form is how an
// unconditional failure is spelled.
//
// Revisions:
//   - 2026-09-20 00:02: initial creation
//   - 2026-10-03 20:56: begins the run itself, since it ends it partway, and reads the
//     refusal from what ending the run answers with
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrCondition, formerly ErrNotACondition (C121, Q348)
func TestAssert_RefusesALoneString(t *testing.T) {
	var (
		thread = &starlark.Thread{
			Name: ENTRY,
		}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			ENTRY,
			nil,
		)
		_, err = _Call(
			t,
			thread,
			spec.ASSERT,
			starlark.String("mistyped"),
		)
	)
	if !errors.Is(
		err,
		core.ErrCondition,
	) {
		t.Fatalf(
			"got %v, want ErrCondition",
			err,
		)
	}

	produced := finish(nil)
	if !errors.Is(
		produced,
		core.ErrCondition,
	) {
		t.Fatalf(
			"the run produced %v, want the refusal",
			produced,
		)
	}

	var (
		bare = &starlark.Thread{
			Name: SCRIPT_NAME,
		}
		kwargs = []starlark.Tuple{{starlark.String("msg"), starlark.String(WHY)}}
	)

	_, err = starlark.Call(
		bare,
		core.Builtins()[spec.ASSERT],
		nil,
		kwargs,
	)
	if !errors.Is(
		err,
		core.ErrAssert,
	) || !strings.Contains(
		err.Error(),
		WHY,
	) {
		t.Fatalf(
			"assert(msg = ...) gave %v, want ErrAssert carrying the message",
			err,
		)
	}

	_, err = _Call(
		t,
		bare,
		spec.ASSERT,
		starlark.String("a truthy condition"),
		starlark.String(WHY),
	)
	if err != nil {
		t.Fatalf(
			"a string condition with a message gave %v, want it to pass",
			err,
		)
	}
}

// TestAssert_StopsTheWholeRun proves a failed assertion is not local to the
// thread that made it: a sibling counting to a hundred million stops in
// milliseconds, and reports cancellation rather than failure.
//
// Revisions:
//   - 2026-09-20 00:14: initial creation
//   - 2026-10-03 20:56: begins the run itself, since it ends it partway, and reads the
//     assertion from what ending the run answers with
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestAssert_StopsTheWholeRun(t *testing.T) {
	var (
		thread = &starlark.Thread{
			Name: ENTRY,
		}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			ENTRY,
			nil,
		)
		spinner = _Spawned(
			t,
			thread,
			SPINNER,
		)
		failing = _Spawned(
			t,
			thread,
			ISOLATED,
		)
		_, err = scheduler.Wait(
			thread,
			failing,
		)
	)
	if err == nil {
		t.Fatal("the asserting thread succeeded")
	}

	select {
	case <-spinner.Done():
	case <-time.After(LIMIT):
		t.Fatal("a sibling thread ran on after the assertion")
	}

	produced := finish(nil)
	if !errors.Is(
		produced,
		core.ErrAssert,
	) {
		t.Fatalf(
			"the run produced %v, want ErrAssert",
			produced,
		)
	}
}

// TestSleep_RefusesWhatIsNotADuration proves sleep reads its argument through
// the one reader every duration goes through.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-10-02 00:48: refuses a fraction of a millisecond
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSleep_RefusesWhatIsNotADuration(t *testing.T) {
	thread := _Begun(
		t.Context(),
		t,
	)

	for _, given := range []starlark.Value{
		starlark.String("1"),
		starlark.MakeInt(-1),
		starlark.Float(0.5),
		starlark.Float(1e300),
	} {
		_, err := _Call(
			t,
			thread,
			common.SLEEP,
			given,
		)
		if !errors.Is(
			err,
			scheduler.ErrDuration,
		) {
			t.Fatalf(
				"sleep(%v) gave %v, want ErrDuration",
				given,
				err,
			)
		}
	}
}

// TestSleep_IsInterruptedByCancellation proves a sleep watches its context,
// which is the rule every blocking builtin owes.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-10-02 00:48: sleeps thirty seconds in milliseconds, which sleep now
//     takes
//   - 2026-10-03 20:56: leaves ending the run to _Begun, which checks what it produced
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 13:25: expects scheduler.ErrCancelled and the run's own context error (C98, Q249)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSleep_IsInterruptedByCancellation(t *testing.T) {
	var (
		ctx, stop = context.WithCancel(t.Context())
		thread    = _Begun(
			ctx,
			t,
		)
		slept = make(
			chan error,
			1,
		)
	)

	go func() {
		_, err := _Call(
			t,
			thread,
			common.SLEEP,
			starlark.MakeInt(LONG_SLEEP),
		)
		slept <- err
	}()

	time.Sleep(STOP_AFTER)
	stop()

	select {
	case err := <-slept:
		if !errors.Is(
			err,
			scheduler.ErrCancelled,
		) || !errors.Is(
			err,
			context.Canceled,
		) {
			t.Fatalf(
				"got %v, want ErrCancelled and context.Canceled",
				err,
			)
		}
	case <-time.After(LIMIT):
		t.Fatal("a cancelled sleep slept on")
	}
}

// _Lines is a reporter that keeps how each line ended, in order. It keeps only
// the ends of the entry thread's lines: a spawned thread's own end arrives
// from its goroutine at a time the test does not control (B105).
//
// Revisions:
//   - 2026-11-02 00:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Lines struct {
	mutex sync.Mutex
	ended []error
}

// Started is empty: the test reads only how a line ended.
//
// Revisions:
//   - 2026-11-02 00:29: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (l *_Lines) Started(
	thread string,
	line *common.Line,
) {
}

// Ended keeps err when the line belongs to the entry thread.
//
// Revisions:
//   - 2026-11-02 00:29: initial creation
//   - 2026-11-05 16:55: keeps only ends of the entry thread's lines (C97, B105)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 01:04: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (l *_Lines) Ended(
	thread string,
	name string,
	err error,
) {
	if thread != common.SPINE {
		return
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.ended = append(
		l.ended,
		err,
	)
}

// _Endings is a copy of every end kept so far.
//
// Revisions:
//   - 2026-11-02 00:29: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (l *_Lines) _Endings() []error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return slices.Clone(l.ended)
}

// TestCancel_EndsItsLineOnceWhicheverWayItGoes proves cancel reports exactly
// one end for its line, carrying the refusal when its arguments are refused
// and nothing when it stops the handles.
//
// Revisions:
//   - 2026-11-02 00:29: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 01:03: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrHandle, formerly ErrNotAHandle (C121, Q348)
func TestCancel_EndsItsLineOnceWhicheverWayItGoes(t *testing.T) {
	var (
		into   = new(_Lines)
		thread = &starlark.Thread{
			Name: ENTRY,
		}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			ENTRY,
			&common.Settings{
				Reporter: into,
			},
		)
	)

	t.Cleanup(func() {
		_ = finish(nil)
	})

	var (
		handle = _Spawned(
			t,
			thread,
			SPINNER,
		)
		tests = []struct {
			name string
			arg  starlark.Value
			want error
		}{
			{
				name: "a number",
				arg:  starlark.MakeInt(1),
				want: core.ErrHandle,
			},
			{
				name: "a handle",
				arg:  handle,
			},
		}
	)

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					before = len(into._Endings())
				)

				_, err := _Call(
					t,
					thread,
					common.CANCEL,
					test.arg,
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

				ended := into._Endings()
				if len(ended) != before+1 {
					t.Fatalf(
						"cancel ended its line %d times",
						len(ended)-before,
					)
				}

				if !errors.Is(
					ended[before],
					test.want,
				) {
					t.Fatalf(
						"line ended with %v, want %v",
						ended[before],
						test.want,
					)
				}
			},
		)
	}
}
