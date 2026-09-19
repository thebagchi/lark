// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: join and cancel are Starlark builtins that need a run on their thread,
// and nothing puts one there until a later phase in another package.
package scheduler

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

const (
	FAILING      = "failing"
	SECOND       = "second"
	ANSWER_TWO   = 9
	JOIN_LIMIT   = 5 * time.Second
	FAILURE_TEXT = "a spawned failure"
	SETTLE_TRIES = 50
	SETTLE_WAIT  = 10 * time.Millisecond
)

// _Joined calls _Join with the handles given.
//
// Revisions:
//   - 2026-09-19 22:10: initial creation
func _Joined(run *_Run, handles ...*Handle) (starlark.Value, error) {
	args := make(starlark.Tuple, 0, len(handles))

	for _, handle := range handles {
		args = append(args, handle)
	}

	return _Join(_Thread(run), nil, args, nil)
}

// TestJoin_ReturnsResultsInArgumentOrder proves a join hands back what each
// thread produced, ordered by how the script asked rather than by which
// finished first.
//
// This is also the first test of the happens-before edge the design rests on:
// _Work writes value on one goroutine and _Join reads it on another, and the
// close of the done channel is the only thing making that safe. Under -race, a
// read that skipped the channel is what the detector would report.
//
// Revisions:
//   - 2026-09-19 22:11: initial creation
func TestJoin_ReturnsResultsInArgumentOrder(t *testing.T) {
	globals := _Globals(t)
	run := _Started(t.Context())

	first, err := _Spawned(t, run, globals[WORKER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	second, err := _Spawned(t, run, globals[SECOND])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	value, err := _Joined(run, first, second)
	if err != nil {
		t.Fatalf("join: %v", err)
	}

	list, ok := value.(*starlark.List)
	if !ok {
		t.Fatalf("join returned %T, want *starlark.List", value)
	}

	want := []int64{ANSWER, ANSWER_TWO}

	if list.Len() != len(want) {
		t.Fatalf("join returned %d results, want %d", list.Len(), len(want))
	}

	for index, expected := range want {
		number, ok := list.Index(index).(starlark.Int)
		if !ok {
			t.Fatalf("result %d is %T, want starlark.Int", index, list.Index(index))
		}

		got, _ := number.Int64()
		if got != expected {
			t.Fatalf("result %d is %d, want %d", index, got, expected)
		}
	}
}

// TestJoin_ReraisesAFailure proves a caller cannot take a result list and be
// unaware that one thread never produced anything.
//
// Revisions:
//   - 2026-09-19 22:12: initial creation
func TestJoin_ReraisesAFailure(t *testing.T) {
	globals := _Globals(t)
	run := _Started(t.Context())

	handle, err := _Spawned(t, run, globals[FAILING])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	_, err = _Joined(run, handle)
	if err == nil {
		t.Fatal("joining a failed thread succeeded")
	}

	if !strings.Contains(err.Error(), FAILING) {
		t.Fatalf("the failure does not name the function: %v", err)
	}

	if !strings.Contains(err.Error(), FAILURE_TEXT) {
		t.Fatalf("the failure does not carry what the script raised: %v", err)
	}

	t.Logf("re-raised at the join: %v", err)
}

// TestJoin_FirstFailureInArgumentOrderWins proves the outcome depends on how
// the script was written, not on which thread lost a race. A failing thread
// passed second must not mask a failing thread passed first.
//
// Revisions:
//   - 2026-09-19 22:13: initial creation
func TestJoin_FirstFailureInArgumentOrderWins(t *testing.T) {
	globals := _Globals(t)
	run := _Started(t.Context())

	slow, err := _Spawned(t, run, globals[SPINNER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	quick, err := _Spawned(t, run, globals[FAILING])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	slow.stop()

	_, err = _Joined(run, slow, quick)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("got %v, want the first argument's failure (ErrCancelled)", err)
	}
}

// TestJoin_AbandonsNothingWhenItGivesUp proves the half of fail-fast that is
// easy to leave out: when a handle fails, the ones join has not reached are
// cancelled *and waited for*, so no evaluation is still unwinding after join
// returns.
//
// It was called TestJoin_WaitsForEveryHandleBeforeReadingAny and asserted that
// every handle finished before any result was read - which is exactly what
// fail-fast removed on 2026-09-20 00:10. It kept passing, because cancelling
// and waiting also leaves every channel closed, so its name and its comment
// described a property the code no longer had.
//
// Revisions:
//   - 2026-09-19 22:14: initial creation
//   - 2026-09-20 00:15: renamed and re-documented for what it actually proves
func TestJoin_AbandonsNothingWhenItGivesUp(t *testing.T) {
	globals := _Globals(t)
	run := _Started(t.Context())

	quick, err := _Spawned(t, run, globals[FAILING])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	slow, err := _Spawned(t, run, globals[WORKER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		_, _ = _Joined(run, quick, slow)
	}()

	select {
	case <-done:
	case <-time.After(JOIN_LIMIT):
		t.Fatal("join did not return")
	}

	select {
	case <-slow.done:
	default:
		t.Fatal("join returned while a handle it was given was still running")
	}
}

// TestCancel_StopsWithoutWaiting proves cancel is not a join: it returns None
// and does not block, and a cancelled handle raises when joined because a
// caller asking for its result is asking for something that will never exist.
//
// Revisions:
//   - 2026-09-19 22:15: initial creation
func TestCancel_StopsWithoutWaiting(t *testing.T) {
	run := _Started(t.Context())

	handle, err := _Spawned(t, run, _Globals(t)[SPINNER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	value, err := _Cancel(_Thread(run), nil, starlark.Tuple{handle}, nil)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if value != starlark.None {
		t.Fatalf("cancel returned %v, want None", value)
	}

	_, err = _Joined(run, handle)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("joining a cancelled handle gave %v, want ErrCancelled", err)
	}
}

// TestCancel_IsIdempotent proves cancelling a handle twice, or one that has
// already finished, is not an error - a script cannot know whether a thread
// stopped a moment ago.
//
// Revisions:
//   - 2026-09-19 22:16: initial creation
func TestCancel_IsIdempotent(t *testing.T) {
	run := _Started(t.Context())

	handle, err := _Spawned(t, run, _Globals(t)[WORKER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	<-handle.done

	for attempt := range 2 {
		_, err = _Cancel(_Thread(run), nil, starlark.Tuple{handle}, nil)
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
	run := _Started(t.Context())

	for _, name := range []string{JOIN, CANCEL} {
		call := _Join
		if name == CANCEL {
			call = _Cancel
		}

		t.Run(name+" a number", func(t *testing.T) {
			_, err := call(_Thread(run), nil, starlark.Tuple{starlark.MakeInt(1)}, nil)
			if !errors.Is(err, ErrNotAHandle) {
				t.Fatalf("got %v, want ErrNotAHandle", err)
			}
		})

		t.Run(name+" a keyword", func(t *testing.T) {
			kwargs := []starlark.Tuple{{starlark.String("x"), starlark.MakeInt(1)}}

			_, err := call(_Thread(run), nil, starlark.Tuple{}, kwargs)
			if !errors.Is(err, ErrNotAHandle) {
				t.Fatalf("got %v, want ErrNotAHandle", err)
			}
		})
	}
}

// TestCancel_EndsTheGoroutineNotJustTheEvaluation proves a cancel reaches all
// the way down: the interpreter stops, starlark.Call returns, _Work returns,
// and both the goroutine running it and the watcher _CancelOn started are gone.
//
// Cancelling the thread and ending the goroutine are not the same thing, and
// only the first is what thread.Cancel does. The rest follows from _Work
// returning, which nothing else tests - group.Wait covers the worker, and the
// goroutine count covers the watcher, which is not in the group.
//
// The count is polled rather than slept on, because a goroutine that has been
// told to stop has not necessarily been scheduled yet.
//
// Revisions:
//   - 2026-09-19 22:14: initial creation
func TestCancel_EndsTheGoroutineNotJustTheEvaluation(t *testing.T) {
	run := _Started(t.Context())

	before := runtime.NumGoroutine()

	handle, err := _Spawned(t, run, _Globals(t)[SPINNER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	_, err = _Cancel(_Thread(run), nil, starlark.Tuple{handle}, nil)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}

	waited := make(chan struct{})

	go func() {
		defer close(waited)

		run.group.Wait()
	}()

	select {
	case <-waited:
	case <-time.After(JOIN_LIMIT):
		t.Fatal("a spawned goroutine outlived the cancel")
	}

	settled := false

	for range SETTLE_TRIES {
		if runtime.NumGoroutine() <= before {
			settled = true

			break
		}

		time.Sleep(SETTLE_WAIT)
	}

	if !settled {
		t.Fatalf(
			"goroutines did not settle: %d before the spawn, %d after the cancel",
			before,
			runtime.NumGoroutine(),
		)
	}
}

// TestSpawn_LeavesNoWatcherBehindWhenItFinishes proves the watcher _CancelOn
// starts is released when the spawned call returns on its own.
//
// It exists because the cancel test above does not catch this: a cancelled
// context makes the watcher exit through its own select, so removing the
// release changes nothing there.
//
// What planting found is that the watcher is released twice over. _Work defers
// the function _CancelOn returns, and the goroutine in _Spawn defers cancelling
// the child context that watcher is waiting on. Either alone ends it, so this
// test only fails when both are removed. That redundancy is real and is
// recorded rather than trimmed: _CancelOn is also phase 8's, where no second
// cancel is guaranteed.
//
// Revisions:
//   - 2026-09-19 22:17: initial creation
func TestSpawn_LeavesNoWatcherBehindWhenItFinishes(t *testing.T) {
	run := _Started(t.Context())

	before := runtime.NumGoroutine()

	handle, err := _Spawned(t, run, _Globals(t)[WORKER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	_, err = _Joined(run, handle)
	if err != nil {
		t.Fatalf("join: %v", err)
	}

	settled := false

	for range SETTLE_TRIES {
		if runtime.NumGoroutine() <= before {
			settled = true

			break
		}

		time.Sleep(SETTLE_WAIT)
	}

	if !settled {
		t.Fatalf(
			"a watcher outlived the call it was started for: %d goroutines before, %d after",
			before,
			runtime.NumGoroutine(),
		)
	}
}
