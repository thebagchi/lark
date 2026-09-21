package core_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/dialect"
	"github.com/thebagchi/lark/runtime/plugin/core"
	"github.com/thebagchi/lark/runtime/scheduler"
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
	FAILURE_TEXT  = "a spawned failure"
	WHY           = "two and two"
	EXPECTED_NAME = "core"
	STOP_AFTER    = 20 * time.Millisecond
	LIMIT         = 5 * time.Second
	SETTLE_TRIES  = 50
	SETTLE_WAIT   = 10 * time.Millisecond
)

// SUPPLIED is every name this plugin puts in a script's environment, written
// once, so the test and the plugin cannot disagree about the count.
var SUPPLIED = []string{core.SPAWN, core.JOIN, core.CANCEL, core.ASSERT, core.SLEEP}

const SOURCE = `
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
`

// _Globals compiles SOURCE against the plugin's names and freezes what it
// produced.
//
// Revisions:
//   - 2026-09-19 22:03: initial creation
//   - 2026-09-21 09:46: through the plugin's own surface
func _Globals(t *testing.T) starlark.StringDict {
	t.Helper()

	globals, err := starlark.ExecFileOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT_NAME},
		SCRIPT_NAME,
		[]byte(SOURCE),
		core.Builtins(),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	globals.Freeze()

	return globals
}

// _Begun returns a thread carrying a fresh run, and the function that ends
// the run, as Evaluate would leave them around a call.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func _Begun(t *testing.T, ctx context.Context) (*starlark.Thread, func()) {
	t.Helper()

	thread := &starlark.Thread{Name: ENTRY}

	return thread, scheduler.Begin(ctx, thread, ENTRY)
}

// _Call invokes one of the plugin's builtins on thread with the values given.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func _Call(t *testing.T, thread *starlark.Thread, name string, args ...starlark.Value) (starlark.Value, error) {
	t.Helper()

	return starlark.Call(thread, core.Builtins()[name], starlark.Tuple(args), nil)
}

// _Spawned spawns the named global and returns the handle.
//
// Revisions:
//   - 2026-09-19 22:04: initial creation
func _Spawned(t *testing.T, thread *starlark.Thread, name string) *scheduler.Handle {
	t.Helper()

	value, err := _Call(t, thread, core.SPAWN, _Globals(t)[name])
	if err != nil {
		t.Fatalf("spawn %s: %v", name, err)
	}

	handle, ok := value.(*scheduler.Handle)
	if !ok {
		t.Fatalf("spawn returned %T, want *scheduler.Handle", value)
	}

	return handle
}

// TestBuiltins_SuppliesItsNames proves what a host gets, and that each call
// builds a fresh map rather than handing out one shared with every other host.
//
// Revisions:
//   - 2026-09-19 22:23: initial creation
//   - 2026-09-21 09:46: on the plugin
func TestBuiltins_SuppliesItsNames(t *testing.T) {
	values := core.Builtins()

	if len(values) != len(SUPPLIED) {
		t.Fatalf("supplies %v, want %v", values.Keys(), SUPPLIED)
	}

	for _, name := range SUPPLIED {
		builtin, ok := values[name].(*starlark.Builtin)
		if !ok {
			t.Fatalf("%s is %T, want *starlark.Builtin", name, values[name])
		}

		if builtin.Name() != name {
			t.Fatalf("%s is named %q", name, builtin.Name())
		}
	}

	values[core.SPAWN] = starlark.None

	if core.Builtins()[core.SPAWN] == starlark.None {
		t.Fatal("changing one host's environment changed everybody's")
	}
}

// TestSpawn_RunsAndJoins proves the whole shape through the surface: a spawn
// returns a handle, and a join returns what it produced, in argument order.
//
// Revisions:
//   - 2026-09-19 22:11: initial creation
func TestSpawn_RunsAndJoins(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	first := _Spawned(t, thread, WORKER)
	second := _Spawned(t, thread, SECOND)

	value, err := _Call(t, thread, core.JOIN, first, second)
	if err != nil {
		t.Fatalf("join: %v", err)
	}

	if value.String() != "[7, 9]" {
		t.Fatalf("join returned %s, want [7, 9]", value)
	}

	if first.Thread() != "thread_1" || second.Thread() != "thread_2" {
		t.Fatalf("lanes are %s and %s", first.Thread(), second.Thread())
	}
}

// TestSpawn_RefusesWhatCannotBeCalled proves each refusal, and that all reach
// one sentinel a host can act on.
//
// Revisions:
//   - 2026-09-19 22:06: initial creation
func TestSpawn_RefusesWhatCannotBeCalled(t *testing.T) {
	globals := _Globals(t)

	thread, finish := _Begun(t, t.Context())
	defer finish()

	cases := []struct {
		name string
		args []starlark.Value
	}{
		{name: "nothing", args: nil},
		{name: "two functions", args: []starlark.Value{globals[WORKER], globals[WORKER]}},
		{name: "not a function", args: []starlark.Value{globals[NOT_A_FUNC]}},
		{name: "takes arguments", args: []starlark.Value{globals[TAKES_ARGS]}},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Call(t, thread, core.SPAWN, item.args...)
			if !errors.Is(err, core.ErrNotAName) {
				t.Fatalf("got %v, want ErrNotAName", err)
			}
		})
	}
}

// TestSpawn_TakesALambda records that a compiler's spawn(lambda: ...) is
// accepted, and named as the interpreter names it.
//
// Revisions:
//   - 2026-09-20 20:53: initial creation
func TestSpawn_TakesALambda(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	if got := _Spawned(t, thread, ANON_FUNC).Name(); got != scheduler.LAMBDA {
		t.Fatalf("want the interpreter's own word for it, got %q", got)
	}
}

// TestSpawn_RefusesAThreadWithNoRun proves spawn called outside a run is told
// so, rather than reaching a nil run.
//
// Revisions:
//   - 2026-09-19 22:07: initial creation
func TestSpawn_RefusesAThreadWithNoRun(t *testing.T) {
	_, err := _Call(t, &starlark.Thread{Name: SCRIPT_NAME}, core.SPAWN, _Globals(t)[WORKER])
	if !errors.Is(err, scheduler.ErrNoRun) {
		t.Fatalf("got %v, want ErrNoRun", err)
	}
}

// TestJoin_ReraisesAFailure proves a caller cannot take a result list and be
// unaware that one thread never produced anything.
//
// Revisions:
//   - 2026-09-19 22:12: initial creation
func TestJoin_ReraisesAFailure(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	_, err := _Call(t, thread, core.JOIN, _Spawned(t, thread, FAILING))
	if err == nil {
		t.Fatal("joining a failed thread succeeded")
	}

	for _, want := range []string{FAILING, FAILURE_TEXT} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the failure does not carry %q: %v", want, err)
		}
	}
}

// TestJoin_FirstFailureInArgumentOrderWins proves the outcome depends on how
// the script was written, not on which thread lost a race.
//
// Revisions:
//   - 2026-09-19 22:13: initial creation
func TestJoin_FirstFailureInArgumentOrderWins(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	slow := _Spawned(t, thread, SPINNER)
	quick := _Spawned(t, thread, FAILING)

	slow.Stop()

	_, err := _Call(t, thread, core.JOIN, slow, quick)
	if !errors.Is(err, scheduler.ErrCancelled) {
		t.Fatalf("got %v, want the first argument's failure (ErrCancelled)", err)
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
func TestJoin_AbandonsNothingWhenItGivesUp(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	quick := _Spawned(t, thread, FAILING)
	slow := _Spawned(t, thread, SPINNER)

	joined := make(chan error, 1)

	go func() {
		_, err := _Call(t, thread, core.JOIN, quick, slow)
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
func TestTimeout_ReachesAJoin(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	target, ok := _Globals(t)[WAITER].(starlark.Callable)
	if !ok {
		t.Fatalf("%s is not callable", WAITER)
	}

	bounded, err := scheduler.Beside(thread, target, scheduler.Inline())
	if err != nil {
		t.Fatalf("beside: %v", err)
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
func TestCancel_StopsWithoutWaiting(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	handle := _Spawned(t, thread, SPINNER)

	value, err := _Call(t, thread, core.CANCEL, handle)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if value != starlark.None {
		t.Fatalf("cancel returned %v, want None", value)
	}

	_, err = _Call(t, thread, core.JOIN, handle)
	if !errors.Is(err, scheduler.ErrCancelled) {
		t.Fatalf("joining a cancelled handle gave %v, want ErrCancelled", err)
	}
}

// TestCancel_IsIdempotent proves cancelling a handle twice, or one that has
// already finished, is not an error.
//
// Revisions:
//   - 2026-09-19 22:16: initial creation
func TestCancel_IsIdempotent(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	handle := _Spawned(t, thread, WORKER)

	if _, err := scheduler.Wait(thread, handle); err != nil {
		t.Fatal(err)
	}

	for attempt := range 2 {
		_, err := _Call(t, thread, core.CANCEL, handle)
		if err != nil {
			t.Fatalf("cancel %d of a finished handle: %v", attempt, err)
		}
	}
}

// TestHandles_RefusesWhatIsNotAHandle proves both builtins read their arguments
// through one guard, and that a keyword argument is refused too.
//
// Revisions:
//   - 2026-09-19 22:17: initial creation
func TestHandles_RefusesWhatIsNotAHandle(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	for _, name := range []string{core.JOIN, core.CANCEL} {
		t.Run(name+" a number", func(t *testing.T) {
			_, err := _Call(t, thread, name, starlark.MakeInt(1))
			if !errors.Is(err, core.ErrNotAHandle) {
				t.Fatalf("got %v, want ErrNotAHandle", err)
			}
		})

		t.Run(name+" a keyword", func(t *testing.T) {
			kwargs := []starlark.Tuple{{starlark.String("x"), starlark.MakeInt(1)}}

			_, err := starlark.Call(thread, core.Builtins()[name], nil, kwargs)
			if !errors.Is(err, core.ErrNotAHandle) {
				t.Fatalf("got %v, want ErrNotAHandle", err)
			}
		})
	}
}

// _Settled polls until the goroutine count is back at or under before, or
// gives up.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
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
func TestRun_LeavesNoGoroutineBehind(t *testing.T) {
	before := runtime.NumGoroutine()

	thread, finish := _Begun(t, t.Context())

	spinner := _Spawned(t, thread, SPINNER)
	worker := _Spawned(t, thread, WORKER)

	if _, err := _Call(t, thread, core.CANCEL, spinner); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := _Call(t, thread, core.JOIN, worker); err != nil {
		t.Fatalf("join: %v", err)
	}

	finish()

	if !_Settled(before) {
		t.Fatalf("goroutines did not settle: %d before, %d after", before, runtime.NumGoroutine())
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
func TestAssert_UsesStarlarkTruthiness(t *testing.T) {
	bare := &starlark.Thread{Name: SCRIPT_NAME}

	for _, value := range []starlark.Value{starlark.MakeInt(0), starlark.NewList(nil), starlark.None} {
		_, err := _Call(t, bare, core.ASSERT, value)
		if !errors.Is(err, core.ErrAssert) {
			t.Fatalf("assert(%v) gave %v, want ErrAssert", value, err)
		}
	}

	value, err := _Call(t, bare, core.ASSERT, starlark.MakeInt(1), starlark.String(WHY))
	if err != nil || value != starlark.None {
		t.Fatalf("assert(1) gave %v, %v", value, err)
	}

	_, err = _Call(t, bare, core.ASSERT, starlark.False, starlark.String(WHY))
	if !errors.Is(err, core.ErrAssert) || !strings.Contains(err.Error(), WHY) {
		t.Fatalf("want ErrAssert carrying the message, got %v", err)
	}
}

// TestAssert_RefusesALoneString proves the trap is closed: assert("text")
// reads like an unconditional failure and used to behave like a passing test.
// The refusal ends the run like any assertion, and the keyword form is how an
// unconditional failure is spelled.
//
// Revisions:
//   - 2026-09-20 00:02: initial creation
func TestAssert_RefusesALoneString(t *testing.T) {
	thread, finish := _Begun(t, t.Context())

	_, err := _Call(t, thread, core.ASSERT, starlark.String("mistyped"))
	if !errors.Is(err, core.ErrNotACondition) {
		t.Fatalf("got %v, want ErrNotACondition", err)
	}

	finish()

	if !errors.Is(scheduler.Outcome(thread), core.ErrNotACondition) {
		t.Fatalf("the run ended with %v, want the refusal", scheduler.Outcome(thread))
	}

	bare := &starlark.Thread{Name: SCRIPT_NAME}

	kwargs := []starlark.Tuple{{starlark.String("msg"), starlark.String(WHY)}}

	_, err = starlark.Call(bare, core.Builtins()[core.ASSERT], nil, kwargs)
	if !errors.Is(err, core.ErrAssert) || !strings.Contains(err.Error(), WHY) {
		t.Fatalf("assert(msg = ...) gave %v, want ErrAssert carrying the message", err)
	}

	_, err = _Call(t, bare, core.ASSERT, starlark.String("a truthy condition"), starlark.String(WHY))
	if err != nil {
		t.Fatalf("a string condition with a message gave %v, want it to pass", err)
	}
}

// TestAssert_StopsTheWholeRun proves a failed assertion is not local to the
// thread that made it: a sibling counting to a hundred million stops in
// milliseconds, and reports cancellation rather than failure.
//
// Revisions:
//   - 2026-09-20 00:14: initial creation
func TestAssert_StopsTheWholeRun(t *testing.T) {
	thread, finish := _Begun(t, t.Context())

	spinner := _Spawned(t, thread, SPINNER)
	failing := _Spawned(t, thread, ISOLATED)

	_, err := scheduler.Wait(thread, failing)
	if err == nil {
		t.Fatal("the asserting thread succeeded")
	}

	select {
	case <-spinner.Done():
	case <-time.After(LIMIT):
		t.Fatal("a sibling thread ran on after the assertion")
	}

	finish()

	if !errors.Is(scheduler.Outcome(thread), core.ErrAssert) {
		t.Fatalf("the run ended with %v, want ErrAssert", scheduler.Outcome(thread))
	}
}

// TestSleep_RefusesWhatIsNotADuration proves sleep reads its argument through
// the one reader every duration goes through.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func TestSleep_RefusesWhatIsNotADuration(t *testing.T) {
	thread, finish := _Begun(t, t.Context())
	defer finish()

	for _, given := range []starlark.Value{starlark.String("1"), starlark.MakeInt(-1), starlark.Float(1e300)} {
		_, err := _Call(t, thread, core.SLEEP, given)
		if !errors.Is(err, scheduler.ErrDuration) {
			t.Fatalf("sleep(%v) gave %v, want ErrDuration", given, err)
		}
	}
}

// TestSleep_IsInterruptedByCancellation proves a sleep watches its context,
// which is the rule every blocking builtin owes.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func TestSleep_IsInterruptedByCancellation(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())

	thread, finish := _Begun(t, ctx)
	defer finish()

	slept := make(chan error, 1)

	go func() {
		_, err := _Call(t, thread, core.SLEEP, starlark.MakeInt(30))
		slept <- err
	}()

	time.Sleep(STOP_AFTER)
	stop()

	select {
	case err := <-slept:
		if !errors.Is(err, core.ErrInterrupted) {
			t.Fatalf("got %v, want ErrInterrupted", err)
		}
	case <-time.After(LIMIT):
		t.Fatal("a cancelled sleep slept on")
	}
}
