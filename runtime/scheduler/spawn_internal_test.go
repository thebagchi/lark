// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: spawn is a Starlark builtin, reachable from a script only once a later
// phase puts it in a predeclared environment, and it needs a run on its thread
// that only a later phase creates. A test here builds both directly.
package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/dialect"
)

const (
	SCRIPT_NAME  = "spawn_test.star"
	WORKER       = "worker"
	SPINNER      = "spin"
	TAKES_ARGS   = "wants"
	NOT_A_FUNC   = "NUMBER"
	ANON_FUNC    = "ANON"
	ANSWER       = 7
	STOP_AFTER   = 20 * time.Millisecond
	CANCEL_LIMIT = 5 * time.Second
	CONCURRENT   = 16
)

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
`

// _Globals compiles SOURCE and freezes what it produced.
//
// Revisions:
//   - 2026-09-19 22:03: initial creation
func _Globals(t *testing.T) starlark.StringDict {
	t.Helper()

	globals, err := starlark.ExecFileOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT_NAME},
		SCRIPT_NAME,
		[]byte(SOURCE),
		(&Builtins{}).Values(),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	globals.Freeze()

	return globals
}

// _Thread returns an interpreter thread carrying run, as a call into an
// artifact would leave one.
//
// Revisions:
//   - 2026-09-19 22:03: initial creation
func _Thread(run *_Run) *starlark.Thread {
	thread := &starlark.Thread{Name: SCRIPT_NAME}
	thread.SetLocal(RUN_KEY, run)
	thread.SetLocal(THREAD_KEY, int32(SPINE))

	return thread
}

// _Spawned calls _Spawn with one argument and returns the handle.
//
// Revisions:
//   - 2026-09-19 22:04: initial creation
func _Spawned(t *testing.T, run *_Run, target starlark.Value) (*Handle, error) {
	t.Helper()

	value, err := _Spawn(_Thread(run), nil, starlark.Tuple{target}, nil)
	if err != nil {
		return nil, err
	}

	handle, ok := value.(*Handle)
	if !ok {
		t.Fatalf("spawn returned %T, want *Handle", value)
	}

	return handle, nil
}

// TestSpawn_RunsOnItsOwnThread proves a spawned function runs, produces a
// value, and publishes it through the done channel.
//
// Revisions:
//   - 2026-09-19 22:05: initial creation
func TestSpawn_RunsOnItsOwnThread(t *testing.T) {
	run := _Started(t.Context())

	handle, err := _Spawned(t, run, _Globals(t)[WORKER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	<-handle.done

	if handle.err != nil {
		t.Fatalf("the spawned call failed: %v", handle.err)
	}

	number, ok := handle.value.(starlark.Int)
	if !ok {
		t.Fatalf("got %T, want starlark.Int", handle.value)
	}

	got, _ := number.Int64()
	if got != ANSWER {
		t.Fatalf("got %d, want %d", got, ANSWER)
	}

	if handle.Thread() != FIRST_SPAWN {
		t.Fatalf("thread is %d, want %d", handle.Thread(), FIRST_SPAWN)
	}
}

// TestSpawn_RefusesWhatCannotBeNamedOrCalled proves each of the five refusals,
// and that all five reach one sentinel a host can act on.
//
// The lambda case is the one with a reason beyond arity: a lambda has no name,
// so a snapshot or a graph would show an anonymous thread.
//
// Revisions:
//   - 2026-09-19 22:06: initial creation
func TestSpawn_RefusesWhatCannotBeNamedOrCalled(t *testing.T) {
	globals := _Globals(t)
	run := _Started(t.Context())

	cases := []struct {
		name string
		args starlark.Tuple
	}{
		{name: "nothing", args: starlark.Tuple{}},
		{name: "two functions", args: starlark.Tuple{globals[WORKER], globals[WORKER]}},
		{name: "not a function", args: starlark.Tuple{globals[NOT_A_FUNC]}},
		{name: "takes arguments", args: starlark.Tuple{globals[TAKES_ARGS]}},
		{name: "a lambda", args: starlark.Tuple{globals[ANON_FUNC]}},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Spawn(_Thread(run), nil, item.args, nil)
			if !errors.Is(err, ErrNotAName) {
				t.Fatalf("got %v, want ErrNotAName", err)
			}
		})
	}
}

// TestSpawn_RefusesAThreadWithNoRun proves spawn called outside a run is told
// so, rather than reaching a nil run.
//
// Revisions:
//   - 2026-09-19 22:07: initial creation
func TestSpawn_RefusesAThreadWithNoRun(t *testing.T) {
	_, err := _Spawn(
		&starlark.Thread{Name: SCRIPT_NAME},
		nil,
		starlark.Tuple{_Globals(t)[WORKER]},
		nil,
	)
	if !errors.Is(err, ErrNoRun) {
		t.Fatalf("got %v, want ErrNoRun", err)
	}
}

// TestSpawn_CancelStopsAnEvaluationMidFlight proves the context reaches the
// spawned interpreter thread: a script counting to a hundred million stops in
// milliseconds, and the failure says it was cancelled rather than wrong.
//
// Revisions:
//   - 2026-09-19 22:08: initial creation
func TestSpawn_CancelStopsAnEvaluationMidFlight(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	run := _Started(ctx)

	handle, err := _Spawned(t, run, _Globals(t)[SPINNER])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	time.Sleep(STOP_AFTER)
	stop()

	select {
	case <-handle.done:
	case <-time.After(CANCEL_LIMIT):
		t.Fatal("a cancelled evaluation ran on")
	}

	if !errors.Is(handle.err, ErrCancelled) {
		t.Fatalf("got %v, want ErrCancelled", handle.err)
	}

	t.Logf("cancelled mid-flight: %v", handle.err)
}

// TestCancelOn_StopsWatchingWhenReleased proves the watcher a spawn leaves
// behind does not outlive the call. Under -race a leaked watcher touching a
// finished thread is what the detector would report.
//
// Revisions:
//   - 2026-09-19 22:09: initial creation
func TestCancelOn_StopsWatchingWhenReleased(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())

	thread := &starlark.Thread{Name: SCRIPT_NAME}

	release := _CancelOn(ctx, thread)
	release()

	stop()

	time.Sleep(STOP_AFTER)
}

// TestSpawn_ConcurrentSpawnsDoNotShareAThread proves the constraint the whole
// design rests on: a starlark.Thread is not safe across goroutines, so every
// spawn gets one of its own while the frozen globals are shared.
//
// It exists because planting a shared thread against the single-spawn tests
// produced no race at all - one evaluation cannot collide with itself. Only
// several evaluations in flight at once can show this, so this test is the only
// thing standing between the design's central claim and an unchecked comment.
//
// Revisions:
//   - 2026-09-19 22:12: initial creation
func TestSpawn_ConcurrentSpawnsDoNotShareAThread(t *testing.T) {
	globals := _Globals(t)
	run := _Started(t.Context())

	handles := make([]*Handle, CONCURRENT)

	for index := range handles {
		handle, err := _Spawned(t, run, globals[WORKER])
		if err != nil {
			t.Fatalf("spawn %d: %v", index, err)
		}

		handles[index] = handle
	}

	seen := map[int32]bool{}

	for index, handle := range handles {
		<-handle.done

		if handle.err != nil {
			t.Fatalf("spawn %d failed: %v", index, handle.err)
		}

		number, ok := handle.value.(starlark.Int)
		if !ok {
			t.Fatalf("spawn %d produced %T, want starlark.Int", index, handle.value)
		}

		got, _ := number.Int64()
		if got != ANSWER {
			t.Fatalf("spawn %d produced %d, want %d", index, got, ANSWER)
		}

		if seen[handle.Thread()] {
			t.Fatalf("thread %d was given to two spawns", handle.Thread())
		}

		seen[handle.Thread()] = true
	}
}
