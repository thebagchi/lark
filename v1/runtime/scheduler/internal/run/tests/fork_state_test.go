// This file tests the state of a spawn and of its run:
//   - TestFork_CountsASpawnDoneOnlyAfterItsCleanup replaces the field
//     Spawn.Cancel, which _Finish calls inside the cleanup, with a function that
//     blocks, and splits a spawn with ForkHandle and ForkGo. A reporter's
//     Ended runs before _Finish, so it cannot hold the cleanup open at that
//     point, and the test reads Run.Group directly;
//   - TestFinish_LetsGoOfTheThreadAndTheRun reads the fields Run, Ctx and Cancel
//     of the Spawn and Locals.ThreadID after the spawn ends;
//   - TestRetention_AKeptHandleReachesNeitherRunNorThread takes weak pointers
//     to a Run and a Flight, and no exported name returns either (the run
//     alone could be watched through a SharedIn value, the flight cannot);
//   - TestRun_ParentSpreadsSpawnsOverShards calls Begun, Parent and Release
//     and reads Run.Shards.
//
// _Started and _Thread build the run and thread these tests use. The tests of
// Fork that read only its results are in fork_test.go.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:45: the tests that need no unexported name moved to the external package; this
//     comment names what stays (C65c)
//   - 2026-11-10 00:25: moved to tests/ from beside_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
//   - 2026-11-12 19:18: header names Fork, ForkHandle, ForkGo and TestFork_*,
//     formerly the Beside names (C129, Q396)
package tests

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
	"weak"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

const (
	THREAD_NAME = "spine"
)

// _Started returns a run as a call into an artifact would leave one, including
// a context it can cancel itself - which is what a failed assertion uses.
//
// Revisions:
//   - 2026-09-19 21:57: initial creation
//   - 2026-09-21 08:09: builds the maps a run now holds
//   - 2026-09-27 01:10: carries a budget, which spawning now charges
//   - 2026-10-29 01:27: leaves the maps nil, as _Begun now does, so a test
//     reaches the path that makes each one
//   - 2026-11-01 13:00: carries no budget
//   - 2026-11-09 01:07: moved to beside_internal_test.go, with the tests that use it (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Started(ctx context.Context) *run.Run {
	inner, stop := context.WithCancel(ctx)

	return &run.Run{
		Ctx:  inner,
		Stop: stop,
	}
}

// _Thread returns an interpreter thread carrying run's spine evaluation, as a
// call into an artifact would leave one.
//
// Revisions:
//   - 2026-09-19 22:03: initial creation
//   - 2026-09-21 08:09: leaves the one local
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-09 01:07: moved to beside_internal_test.go, with the tests that use it (C65c)
//   - 2026-11-10 00:25: moved to tests/, with the local run renamed (C120a, Q311, Q321)
func _Thread(active *run.Run) *starlark.Thread {
	thread := &starlark.Thread{
		Name: THREAD_NAME,
	}
	thread.SetLocal(
		run.LOCALS_KEY,
		&run.Locals{
			Run:      active,
			ThreadID: common.SPINE,
			Ctx:      active.Ctx,
		},
	)

	return thread
}

// TestFork_CountsASpawnDoneOnlyAfterItsCleanup proves the run's wait group
// is released only after the spawn's deferred cleanup has finished. The stop
// of the handle blocks on a gate, so the cleanup cannot finish until the test
// opens it.
//
// The "not released" check uses a short bound as a liveness guard only. It
// cannot fail a correct build: the gate is shut, so Wait cannot return and the
// bound always expires. A build that releases early returns at once, so the
// check fails.
//
// Gap: this pins Done after stop and so after the whole cleanup. It cannot
// pin close(done) apart from Done, because close has no hook.
//
// Revisions:
//   - 2026-11-02 00:19: initial creation, by repeating many spawns
//   - 2026-11-02 00:19: gated stop in place of the repeat, so the check does
//     not depend on timing to catch an early Done
//   - 2026-11-08 19:23: reads the handle through its methods (C112, Q207)
//   - 2026-11-10 00:25: moved to tests/, with the local run renamed (C120a, Q311, Q321)
//   - 2026-11-12 18:58: renamed from TestBeside_CountsASpawnDoneOnlyAfterItsCleanup (C129, Q396)
func TestFork_CountsASpawnDoneOnlyAfterItsCleanup(t *testing.T) {
	var (
		active       = _Started(t.Context())
		thread       = _Thread(active)
		parent, err  = run.Of(thread)
		entered      = make(chan struct{})
		gate         = make(chan struct{})
		released     = make(chan struct{})
		holdsCleanup = 100 * time.Millisecond
	)
	if err != nil {
		t.Fatal(err)
	}

	spawn, flight, refused := run.ForkHandle(
		thread,
		parent,
		_Callable(
			t,
			WORKER,
		),
		nil,
	)
	if refused != nil {
		t.Fatal(refused)
	}

	spawn.Cancel = func() {
		close(entered)
		<-gate
	}

	run.ForkGo(
		spawn,
		flight,
	)

	select {
	case <-entered:
	case <-time.After(CANCEL_LIMIT):
		t.Fatal("the cleanup never started")
	}

	go func() {
		active.Group.Wait()
		close(released)
	}()

	select {
	case <-released:
		t.Fatal("the group was released while the cleanup was running")
	case <-time.After(holdsCleanup):
	}

	close(gate)

	select {
	case <-released:
	case <-time.After(CANCEL_LIMIT):
		t.Fatal("the group was not released after the cleanup")
	}

	select {
	case <-spawn.Chan():
	default:
		t.Fatal("the group was released before done closed")
	}
}

// TestFinish_LetsGoOfTheThreadAndTheRun proves a finished spawn keeps only its
// handle: the context's stop, the locals' run and context, and the target are
// gone, while the name, the thread id and the result stay.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation, for the Q13 decision of change C33
//   - 2026-11-08 19:23: reads the handle through its methods (C112, Q207)
//   - 2026-11-10 00:25: moved to tests/, with the local run renamed (C120a, Q311, Q321)
//   - 2026-11-12 18:58: uses ForkHandle and ForkGo, formerly BesideHandle and BesideGo (C129,
//     Q396)
func TestFinish_LetsGoOfTheThreadAndTheRun(t *testing.T) {
	var (
		active      = _Started(t.Context())
		thread      = _Thread(active)
		parent, err = run.Of(thread)
	)
	if err != nil {
		t.Fatal(err)
	}

	spawn, flight, refused := run.ForkHandle(
		thread,
		parent,
		_Callable(
			t,
			WORKER,
		),
		nil,
	)
	if refused != nil {
		t.Fatal(refused)
	}

	if spawn.Run == nil || spawn.Ctx == nil || spawn.Cancel == nil {
		t.Fatal("the spawn holds no run, context or stop before it starts")
	}

	run.ForkGo(
		spawn,
		flight,
	)

	<-spawn.Chan()

	for name, held := range map[string]bool{
		"stop":    spawn.Cancel != nil,
		"run":     spawn.Run != nil,
		"context": spawn.Ctx != nil,
		"lane":    spawn.ThreadID != "",
	} {
		if held {
			t.Errorf(
				"a finished spawn still holds its %s",
				name,
			)
		}
	}

	if spawn.Value == nil || spawn.Value.String() != ANSWER {
		t.Errorf(
			"the result is %v, want %s",
			spawn.Value,
			ANSWER,
		)
	}

	if spawn.Name() != WORKER || spawn.Thread() == "" {
		t.Errorf(
			"the handle lost its name or thread: %q %q",
			spawn.Name(),
			spawn.Thread(),
		)
	}

	// Stopping a finished handle does nothing, as it always did.
	spawn.Stop()
}

const (
	// COLLECTIONS is how many collections the retention test forces before it
	// asks a weak pointer whether its object is gone.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	COLLECTIONS = 4
)

// _Finished runs one spawn to its end and returns its handle with weak pointers
// to the run and to the flight.
//
// The run and the flight go out of scope here, so only what the handle reaches
// can keep them. Nothing in this function names them after it returns.
//
// Revisions:
//   - 2026-11-02 15:48: initial creation, for the Q13 decision of change C33
//   - 2026-11-08 19:23: reads the handle through its methods (C112, Q207)
//   - 2026-11-10 00:25: moved to tests/, with the local run renamed (C120a, Q311, Q321)
//   - 2026-11-12 18:58: uses ForkHandle and ForkGo, formerly BesideHandle and BesideGo (C129,
//     Q396)
func _Finished(
	t *testing.T,
) (*run.Handle, weak.Pointer[run.Run], weak.Pointer[run.Flight]) {
	t.Helper()

	var (
		active      = _Started(t.Context())
		thread      = _Thread(active)
		parent, err = run.Of(thread)
	)
	if err != nil {
		t.Fatal(err)
	}

	spawn, flight, refused := run.ForkHandle(
		thread,
		parent,
		_Callable(
			t,
			WORKER,
		),
		nil,
	)
	if refused != nil {
		t.Fatal(refused)
	}

	run.ForkGo(
		spawn,
		flight,
	)

	<-spawn.Chan()
	active.Group.Wait()

	return &spawn.Handle, weak.Make(active), weak.Make(flight)
}

// TestRetention_AKeptHandleReachesNeitherRunNorThread proves a finished handle
// keeps no path to the run it came from or to its thread and flight.
//
// A weak pointer to the run and one to the flight are read after collections,
// while the handle is still alive. A handle that kept the locals would reach
// the run through them, and the run would stay. This is the check of Q13; the
// byte count of the runtime test cannot tell the two apart, since the zeroed
// struct is the same size either way.
//
// Check by commenting out the zeroing of the locals in _Finish: the test must
// then fail.
//
// Revisions:
//   - 2026-11-02 15:48: initial creation, for the Q13 decision of change C33
//   - 2026-11-10 00:25: moved to tests/, with the local run renamed (C120a, Q311, Q321)
func TestRetention_AKeptHandleReachesNeitherRunNorThread(t *testing.T) {
	handle, weakRun, flight := _Finished(t)

	for range COLLECTIONS {
		runtime.GC()
	}

	if weakRun.Value() != nil {
		t.Error("a finished handle still reaches its run")
	}

	if flight.Value() != nil {
		t.Error("a finished handle still reaches its flight")
	}

	runtime.KeepAlive(handle)
}

// TestRun_ParentSpreadsSpawnsOverShards calls Parent from many goroutines and
// checks the contexts it hands out.
//
// Success: the first len(shards) calls return the run's context; later calls
// return one of at most len(shards) other contexts; all of them are cancelled
// when the run's context is, and Release is safe to call after.
//
// Revisions:
//   - 2026-11-05 08:20: initial creation (C53)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/, with the local run renamed (C120a, Q311, Q321)
func TestRun_ParentSpreadsSpawnsOverShards(t *testing.T) {
	const CALLS = 1000

	var (
		inner, stop = context.WithCancel(t.Context())
		active      = run.Begun(
			inner,
			stop,
			new(common.Settings),
		)
		got = make(
			[]context.Context,
			CALLS,
		)
		wait sync.WaitGroup
	)

	defer stop()

	for idx := range CALLS {
		wait.Go(func() {
			got[idx] = active.Parent()
		})
	}

	wait.Wait()

	var (
		direct int
		others = map[context.Context]struct{}{}
	)

	for _, ctx := range got {
		if ctx == inner {
			direct++

			continue
		}

		others[ctx] = struct{}{}
	}

	if direct != len(active.Shards) {
		t.Fatalf(
			"%d direct, want %d",
			direct,
			len(active.Shards),
		)
	}

	if len(others) == 0 || len(others) > len(active.Shards) {
		t.Fatalf(
			"%d shards, want 1 to %d",
			len(others),
			len(active.Shards),
		)
	}

	stop()

	for ctx := range others {
		if ctx.Err() == nil {
			t.Fatal("a shard outlived the run's context")
		}
	}

	active.Release()
}
