package tests

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// STOP_AFTER is how long a test lets a spinner run before it cancels, and
	// CANCEL_LIMIT is how long it waits for the cancel to show.
	//
	// Revisions:
	//   - 2026-11-08 22:45: initial creation, as the external form of the constants in
	//     beside_internal_test.go (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	STOP_AFTER   = 20 * time.Millisecond
	CANCEL_LIMIT = 5 * time.Second

	// CONCURRENT is how many evaluations or stops the concurrent tests start.
	//
	// Revisions:
	//   - 2026-11-08 22:45: initial creation, as the external form of the constant in
	//     beside_internal_test.go (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	CONCURRENT = 16
)

const (
	BINDING_NAME = "h1"
	SPAWN_WORD   = "spawn"
)

const (
	// SPAWN_ALLOCS is the most objects one Fork and Wait may allocate. It was
	// 19 before change C33 put the handle, the locals, the line, the watch and the
	// thread in two objects instead of six.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:45: moved to the external package (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
	SPAWN_ALLOCS = 15

	// SPAWN_ROUNDS is how many spawns the allocation count averages over.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:45: moved to the external package (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPAWN_ROUNDS = 200

	// FINISH_ROUNDS is how many times the race test spawns and stops.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:45: moved to the external package (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FINISH_ROUNDS = 200
)

// _Recording is a reporter that keeps what it was told.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Recording struct {
	guard   sync.Mutex
	starts  []*_Start
	endings []string
}

// _Start is one start a reporter was told of.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Start struct {
	thread string
	line   *common.Line
}

// Started keeps the start.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (r *_Recording) Started(
	thread string,
	line *common.Line,
) {
	r.guard.Lock()
	defer r.guard.Unlock()

	r.starts = append(
		r.starts,
		&_Start{
			thread: thread,
			line:   line,
		},
	)
}

// Ended keeps the thread and the name of the end.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (r *_Recording) Ended(
	thread string,
	name string,
	err error,
) {
	r.guard.Lock()
	defer r.guard.Unlock()

	r.endings = append(
		r.endings,
		thread+" "+name,
	)
}

// _Started is every start kept so far.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (r *_Recording) _Started() []*_Start {
	r.guard.Lock()
	defer r.guard.Unlock()

	return append(
		[]*_Start(nil),
		r.starts...,
	)
}

// Ended is every end kept so far.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (r *_Recording) _Ended() []string {
	r.guard.Lock()
	defer r.guard.Unlock()

	return append(
		[]string(nil),
		r.endings...,
	)
}

// TestFork_RunsOnItsOwnLane proves an evaluation runs, produces a value,
// and takes the caller's first child lane.
//
// Revisions:
//   - 2026-09-19 22:05: initial creation
//   - 2026-09-21 09:46: through Beside and Wait
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: renamed from TestBeside_RunsOnItsOwnLane (C129, Q396)
func TestFork_RunsOnItsOwnLane(t *testing.T) {
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

	value, err := run.Wait(
		thread,
		handle,
	)
	if err != nil {
		t.Fatalf(
			"the evaluation failed: %v",
			err,
		)
	}

	if value.String() != ANSWER {
		t.Fatalf(
			"got %s, want %s",
			value,
			ANSWER,
		)
	}

	if handle.Thread() != spelling.Child(
		common.SPINE,
		run.FIRST_SPAWN,
	) {
		t.Fatalf(
			"thread is %s, want %s",
			handle.Thread(),
			spelling.Child(
				common.SPINE,
				run.FIRST_SPAWN,
			),
		)
	}
}

// TestFork_InlineStaysOnTheCallersLane proves a wrapper's evaluation is
// reported where its caller is.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: renamed from TestBeside_InlineStaysOnTheCallersLane (C129, Q396)
func TestFork_InlineStaysOnTheCallersLane(t *testing.T) {
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
			run.Inline(),
		)
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	if handle.Thread() != common.SPINE {
		t.Fatalf(
			"an inline evaluation ran on %s, want the caller's lane",
			handle.Thread(),
		)
	}

	if _, err := run.Wait(
		thread,
		handle,
	); err != nil {
		t.Fatal(err)
	}
}

// TestFork_RefusesAThreadWithNoRun proves an evaluation started outside a
// run is told so, rather than reaching a nil run.
//
// Revisions:
//   - 2026-09-19 22:07: initial creation
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
//   - 2026-11-12 18:58: renamed from TestBeside_RefusesAThreadWithNoRun (C129, Q396)
func TestFork_RefusesAThreadWithNoRun(t *testing.T) {
	_, err := run.Fork(
		&starlark.Thread{
			Name: SCRIPT_NAME,
		},
		_Callable(
			t,
			WORKER,
		),
	)
	if !errors.Is(
		err,
		run.ErrRun,
	) {
		t.Fatalf(
			"got %v, want ErrRun",
			err,
		)
	}
}

// TestFork_CancelStopsAnEvaluationMidFlight proves the context reaches the
// interpreter thread: a script counting to a hundred million stops in
// milliseconds, and the failure says it was cancelled rather than wrong.
//
// Revisions:
//   - 2026-09-19 22:08: initial creation
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: renamed from TestBeside_CancelStopsAnEvaluationMidFlight (C129, Q396)
func TestFork_CancelStopsAnEvaluationMidFlight(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	var (
		thread, _ = _Open(
			ctx,
			t,
			nil,
		)
		handle, err = run.Fork(
			thread,
			_Callable(
				t,
				SPINNER,
			),
		)
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	time.Sleep(STOP_AFTER)
	stop()

	select {
	case <-handle.Done():
	case <-time.After(CANCEL_LIMIT):
		t.Fatal("a cancelled evaluation ran on")
	}

	_, _, failed := handle.Result()
	if !errors.Is(
		failed,
		run.ErrCancelled,
	) {
		t.Fatalf(
			"got %v, want ErrCancelled",
			failed,
		)
	}
}

// TestWait_ReturnsWhenTheWaiterIsCancelled proves a wait watches its own
// evaluation and not only the handle: a thread of one run waiting on a handle
// of another returns when its own run is cancelled, while the handle runs on.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestWait_ReturnsWhenTheWaiterIsCancelled(t *testing.T) {
	ownerCtx, stopOwner := context.WithCancel(t.Context())
	defer stopOwner()

	var (
		owner, _ = _Open(
			ownerCtx,
			t,
			nil,
		)
		spinner, err = run.Fork(
			owner,
			_Callable(
				t,
				SPINNER,
			),
		)
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	var (
		ctx, stop = context.WithCancel(t.Context())
		waiter, _ = _Open(
			ctx,
			t,
			nil,
		)
		waited = make(
			chan error,
			1,
		)
	)

	go func() {
		_, err := run.Wait(
			waiter,
			spinner,
		)
		waited <- err
	}()

	time.Sleep(STOP_AFTER)
	stop()

	select {
	case err := <-waited:
		if !errors.Is(
			err,
			run.ErrCancelled,
		) {
			t.Fatalf(
				"got %v, want ErrCancelled",
				err,
			)
		}
	case <-time.After(CANCEL_LIMIT):
		t.Fatal("a cancelled waiter waited the handle out")
	}

	select {
	case <-spinner.Done():
		t.Fatal("cancelling the waiter stopped a handle it did not own")
	default:
	}
}

// TestFork_ConcurrentEvaluationsDoNotShareAThread proves the constraint the
// whole design rests on: a starlark.Thread is not safe across goroutines, so
// every evaluation gets one of its own while the frozen globals are shared.
//
// Revisions:
//   - 2026-09-19 22:12: initial creation
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: renamed from TestBeside_ConcurrentEvaluationsDoNotShareAThread (C129,
//     Q396)
func TestFork_ConcurrentEvaluationsDoNotShareAThread(t *testing.T) {
	var (
		thread, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		target = _Callable(
			t,
			WORKER,
		)
		handles = make(
			[]common.Handle,
			CONCURRENT,
		)
	)

	for index := range handles {
		handle, err := run.Fork(
			thread,
			target,
		)
		if err != nil {
			t.Fatalf(
				"fork %d: %v",
				index,
				err,
			)
		}

		handles[index] = handle
	}

	seen := map[string]bool{}

	for index, handle := range handles {
		value, err := run.Wait(
			thread,
			handle,
		)
		if err != nil {
			t.Fatalf(
				"evaluation %d failed: %v",
				index,
				err,
			)
		}

		if value.String() != ANSWER {
			t.Fatalf(
				"evaluation %d produced %s",
				index,
				value,
			)
		}

		if seen[handle.Thread()] {
			t.Fatalf(
				"thread %s was given to two evaluations",
				handle.Thread(),
			)
		}

		seen[handle.Thread()] = true
	}
}

// TestFork_ALambdaNobodyLabelledHasNoName records what a handle of an
// anonymous function is called when no spawn labelled it: nothing, since
// "lambda" would read like a function of that name.
//
// Revisions:
//   - 2026-09-20 20:53: initial creation, as
//     TestBeside_NamesALambdaAsTheInterpreterDoes
//   - 2026-10-02 00:42: a lambda nobody labelled has no name, now that no
//     graph resolves one
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: renamed from TestBeside_ALambdaNobodyLabelledHasNoName (C129, Q396)
func TestFork_ALambdaNobodyLabelledHasNoName(t *testing.T) {
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
				ANON_FUNC,
			),
		)
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	if handle.Name() != "" {
		t.Fatalf(
			"want no name, got %q",
			handle.Name(),
		)
	}

	if _, err := run.Wait(
		thread,
		handle,
	); err != nil {
		t.Fatal(err)
	}
}

// TestFork_ReportsASpawnOnTheThreadThatWroteIt checks the line a spawn
// reports: on the parent, labelled with the function the lambda calls, the
// child's id and the binding - and that the child's end is reported on the
// child.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: renamed from TestBeside_ReportsASpawnOnTheThreadThatWroteIt (C129, Q396)
func TestFork_ReportsASpawnOnTheThreadThatWroteIt(t *testing.T) {
	var (
		into        = new(_Recording)
		thread, end = _Open(
			t.Context(),
			t,
			&common.Settings{
				Reporter: into,
			},
		)
		handle, err = run.Fork(
			thread,
			_Callable(
				t,
				ANON_FUNC,
			),
			run.Labelled(
				WORKER,
				BINDING_NAME,
			),
		)
	)
	if err != nil {
		t.Fatalf(
			"fork: %v",
			err,
		)
	}

	if _, err := run.Wait(
		thread,
		handle,
	); err != nil {
		t.Fatal(err)
	}

	end()

	// Begin reports the spine first; the spawn is the second start.
	started := into._Started()
	if len(started) != 2 {
		t.Fatalf(
			"want the spine and one spawn, got %v",
			started,
		)
	}

	var (
		got  = started[1]
		want = &common.Line{
			Name:    WORKER,
			Builtin: SPAWN_WORD,
			Child:   handle.Thread(),
			Binding: BINDING_NAME,
		}
	)

	if got.thread != common.SPINE || got.line.Name != want.Name ||
		got.line.Builtin != want.Builtin ||
		got.line.Child != want.Child || got.line.Binding != want.Binding {
		t.Fatalf(
			"got %s %+v, want %s %+v",
			got.thread,
			got.line,
			common.SPINE,
			want,
		)
	}

	// Exactly two ends: the child's, on the child, and the spine's.
	var (
		ended     = into._Ended()
		wantEnded = []string{
			handle.Thread() + " " + WORKER,
			common.SPINE + " " + common.ENTRY,
		}
	)

	slices.Sort(ended)
	slices.Sort(wantEnded)

	if !slices.Equal(
		ended,
		wantEnded,
	) {
		t.Fatalf(
			"want the child's end on the child and the spine's, got %v",
			ended,
		)
	}
}

// TestFork_AllocatesAtMostTheObjectsOfOneSpawn proves a spawn's handle,
// locals, line and thread share objects, by counting the allocations of one
// Fork and Wait on a function that returns at once.
//
// Starting condition: a run with no reporter, so the count is the scheduler's
// own. Check by changing SPAWN_ALLOCS down by one: the test must then fail.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation, for change C33
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: renamed from TestBeside_AllocatesAtMostTheObjectsOfOneSpawn (C129, Q396)
func TestFork_AllocatesAtMostTheObjectsOfOneSpawn(t *testing.T) {
	var (
		thread, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		target = _Callable(
			t,
			WORKER,
		)
		failure error
	)

	allocs := testing.AllocsPerRun(
		SPAWN_ROUNDS,
		func() {
			handle, err := run.Fork(
				thread,
				target,
			)
			if err != nil {
				failure = err

				return
			}

			_, failure = run.Wait(
				thread,
				handle,
			)
		},
	)

	if failure != nil {
		t.Fatal(failure)
	}

	t.Logf(
		"allocs per spawn: %v",
		allocs,
	)

	if allocs > SPAWN_ALLOCS {
		t.Fatalf(
			"a spawn allocated %v objects, want at most %d",
			allocs,
			SPAWN_ALLOCS,
		)
	}
}

// TestFinish_StopRacesTheFinishSafely proves a Stop from another goroutine, at
// any moment of the spawn's end, reads no half-cleared handle. Run it with
// -race: the detector is the check.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation, for the Q13 decision of change C33
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestFinish_StopRacesTheFinishSafely(t *testing.T) {
	var (
		thread, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		target = _Callable(
			t,
			WORKER,
		)
	)

	for range FINISH_ROUNDS {
		handle, err := run.Fork(
			thread,
			target,
		)
		if err != nil {
			t.Fatal(err)
		}

		var group sync.WaitGroup

		for range CONCURRENT {
			group.Go(func() {
				handle.Stop()
			})
		}

		group.Wait()
		<-handle.Done()

		handle.Stop()
	}
}

// TestFork_CancelReachesEverySpawnUnderTheShards starts more spawns than the
// run has shards, all spinning, cancels the run, and waits for them all.
//
// Success: every handle is done inside CANCEL_LIMIT with ErrCancelled, and a
// spawn made afterwards is refused. Proves a shard passes the run's
// cancellation on to its children.
//
// Revisions:
//   - 2026-11-05 08:20: initial creation (C53)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified, and a handle is read
//     as *Handle (C91w5)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: renamed from TestBeside_CancelReachesEverySpawnUnderTheShards (C129, Q396)
func TestFork_CancelReachesEverySpawnUnderTheShards(t *testing.T) {
	const SPAWNS = 100

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	var (
		thread, _ = _Open(
			ctx,
			t,
			nil,
		)
		handles = make(
			[]common.Handle,
			0,
			SPAWNS,
		)
	)

	for range SPAWNS {
		handle, err := run.Fork(
			thread,
			_Callable(
				t,
				SPINNER,
			),
		)
		if err != nil {
			t.Fatalf(
				"fork: %v",
				err,
			)
		}

		handles = append(
			handles,
			handle,
		)
	}

	time.Sleep(STOP_AFTER)
	stop()

	for idx, handle := range handles {
		select {
		case <-handle.Done():
		case <-time.After(CANCEL_LIMIT):
			t.Fatalf(
				"spawn %d ran on after the run was cancelled",
				idx,
			)
		}

		_, _, failed := handle.Result()
		if !errors.Is(
			failed,
			run.ErrCancelled,
		) {
			t.Fatalf(
				"spawn %d: got %v, want ErrCancelled",
				idx,
				failed,
			)
		}
	}

	_, err := run.Fork(
		thread,
		_Callable(
			t,
			WORKER,
		),
	)
	if !errors.Is(
		err,
		run.ErrCancelled,
	) {
		t.Fatalf(
			"spawn after cancel: got %v, want ErrCancelled",
			err,
		)
	}
}

// TestCancelOn_StopsWatchingWhenReleased proves the watcher a spawn leaves
// behind does not outlive the call: a context cancelled after the release no
// longer cancels the thread, which still runs a function afterwards.
//
// Revisions:
//   - 2026-09-19 22:09: initial creation
//   - 2026-09-21 08:09: asserts the thread survives, rather than sleeping
//   - 2026-10-28 22:36: releases through the watch _Arm now returns
//   - 2026-11-09 01:07: moved to the external package; the watch is the one
//     run.Begin arms and its finish releases (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCancelOn_StopsWatchingWhenReleased(t *testing.T) {
	var (
		ctx, stop   = context.WithCancel(t.Context())
		thread, end = _Open(
			ctx,
			t,
			nil,
		)
	)

	end()

	stop()

	time.Sleep(STOP_AFTER)

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
			"a released watcher still cancelled the thread: %v",
			err,
		)
	}
}
