// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: what Beside leaves on a handle, and what a run records, are unexported.
// Everything a script can do with these is tested through the surface in
// plugin/core, where the builtins live.
package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

const (
	SCRIPT_NAME  = "beside_test.star"
	WORKER       = "worker"
	SPINNER      = "spin"
	ANON_FUNC    = "ANON"
	ANSWER       = "7"
	STOP_AFTER   = 20 * time.Millisecond
	CANCEL_LIMIT = 5 * time.Second
	CONCURRENT   = 16
)

const SOURCE = `
ANON = lambda: 1

def worker():
    return 7

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
		starlark.StringDict{},
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	globals.Freeze()

	return globals
}

// _Callable is the named global as something Beside takes.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func _Callable(t *testing.T, name string) starlark.Callable {
	t.Helper()

	target, ok := _Globals(t)[name].(starlark.Callable)
	if !ok {
		t.Fatalf("%s is not callable", name)
	}

	return target
}

// TestBeside_RunsOnItsOwnLane proves an evaluation runs, produces a value,
// and takes the caller's first child lane.
//
// Revisions:
//   - 2026-09-19 22:05: initial creation
//   - 2026-09-21 09:46: through Beside and Wait
func TestBeside_RunsOnItsOwnLane(t *testing.T) {
	run := _Started(t.Context())

	handle, err := Beside(_Thread(run), _Callable(t, WORKER))
	if err != nil {
		t.Fatalf("beside: %v", err)
	}

	value, err := Wait(_Thread(run), handle)
	if err != nil {
		t.Fatalf("the evaluation failed: %v", err)
	}

	if value.String() != ANSWER {
		t.Fatalf("got %s, want %s", value, ANSWER)
	}

	if handle.Thread() != _Child(SPINE, FIRST_SPAWN) {
		t.Fatalf("thread is %s, want %s", handle.Thread(), _Child(SPINE, FIRST_SPAWN))
	}
}

// TestBeside_InlineStaysOnTheCallersLane proves a wrapper's evaluation is
// reported where its caller is.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func TestBeside_InlineStaysOnTheCallersLane(t *testing.T) {
	run := _Started(t.Context())

	handle, err := Beside(_Thread(run), _Callable(t, WORKER), Inline())
	if err != nil {
		t.Fatalf("beside: %v", err)
	}

	if handle.Thread() != SPINE {
		t.Fatalf("an inline evaluation ran on %s, want the caller's lane", handle.Thread())
	}

	if _, err := Wait(_Thread(run), handle); err != nil {
		t.Fatal(err)
	}
}

// TestBeside_RefusesAThreadWithNoRun proves an evaluation started outside a
// run is told so, rather than reaching a nil run.
//
// Revisions:
//   - 2026-09-19 22:07: initial creation
func TestBeside_RefusesAThreadWithNoRun(t *testing.T) {
	_, err := Beside(&starlark.Thread{Name: SCRIPT_NAME}, _Callable(t, WORKER))
	if !errors.Is(err, ErrNoRun) {
		t.Fatalf("got %v, want ErrNoRun", err)
	}
}

// TestBeside_CancelStopsAnEvaluationMidFlight proves the context reaches the
// interpreter thread: a script counting to a hundred million stops in
// milliseconds, and the failure says it was cancelled rather than wrong.
//
// Revisions:
//   - 2026-09-19 22:08: initial creation
func TestBeside_CancelStopsAnEvaluationMidFlight(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	run := _Started(ctx)

	handle, err := Beside(_Thread(run), _Callable(t, SPINNER))
	if err != nil {
		t.Fatalf("beside: %v", err)
	}

	time.Sleep(STOP_AFTER)
	stop()

	select {
	case <-handle.Done():
	case <-time.After(CANCEL_LIMIT):
		t.Fatal("a cancelled evaluation ran on")
	}

	if !errors.Is(handle.err, ErrCancelled) {
		t.Fatalf("got %v, want ErrCancelled", handle.err)
	}
}

// TestWait_ReturnsWhenTheWaiterIsCancelled proves a wait watches its own
// evaluation and not only the handle: a thread of one run waiting on a handle
// of another returns when its own run is cancelled, while the handle runs on.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestWait_ReturnsWhenTheWaiterIsCancelled(t *testing.T) {
	owner := _Started(t.Context())
	defer owner.stop()

	spinner, err := Beside(_Thread(owner), _Callable(t, SPINNER))
	if err != nil {
		t.Fatalf("beside: %v", err)
	}

	ctx, stop := context.WithCancel(t.Context())

	waiter := _Started(ctx)

	waited := make(chan error, 1)

	go func() {
		_, err := Wait(_Thread(waiter), spinner)
		waited <- err
	}()

	time.Sleep(STOP_AFTER)
	stop()

	select {
	case err := <-waited:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("got %v, want ErrCancelled", err)
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

// TestCancelOn_StopsWatchingWhenReleased proves the watcher a spawn leaves
// behind does not outlive the call: a context cancelled after the release no
// longer cancels the thread, which still runs a function afterwards.
//
// Revisions:
//   - 2026-09-19 22:09: initial creation
//   - 2026-09-21 08:09: asserts the thread survives, rather than sleeping
func TestCancelOn_StopsWatchingWhenReleased(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())

	thread := &starlark.Thread{Name: SCRIPT_NAME}

	release := _CancelOn(ctx, thread)
	release()

	stop()

	time.Sleep(STOP_AFTER)

	_, err := starlark.Call(thread, _Callable(t, WORKER), nil, nil)
	if err != nil {
		t.Fatalf("a released watcher still cancelled the thread: %v", err)
	}
}

// TestBeside_ConcurrentEvaluationsDoNotShareAThread proves the constraint the
// whole design rests on: a starlark.Thread is not safe across goroutines, so
// every evaluation gets one of its own while the frozen globals are shared.
//
// Revisions:
//   - 2026-09-19 22:12: initial creation
func TestBeside_ConcurrentEvaluationsDoNotShareAThread(t *testing.T) {
	run := _Started(t.Context())
	target := _Callable(t, WORKER)

	handles := make([]*Handle, CONCURRENT)

	for index := range handles {
		handle, err := Beside(_Thread(run), target)
		if err != nil {
			t.Fatalf("beside %d: %v", index, err)
		}

		handles[index] = handle
	}

	seen := map[string]bool{}

	for index, handle := range handles {
		value, err := Wait(_Thread(run), handle)
		if err != nil {
			t.Fatalf("evaluation %d failed: %v", index, err)
		}

		if value.String() != ANSWER {
			t.Fatalf("evaluation %d produced %s", index, value)
		}

		if seen[handle.Thread()] {
			t.Fatalf("thread %s was given to two evaluations", handle.Thread())
		}

		seen[handle.Thread()] = true
	}
}

// TestBeside_NamesALambdaAsTheInterpreterDoes records what a handle of an
// anonymous function is called, which a reporter resolves against a graph.
//
// Revisions:
//   - 2026-09-20 20:53: initial creation
func TestBeside_NamesALambdaAsTheInterpreterDoes(t *testing.T) {
	run := _Started(t.Context())

	handle, err := Beside(_Thread(run), _Callable(t, ANON_FUNC))
	if err != nil {
		t.Fatalf("beside: %v", err)
	}

	if handle.Name() != LAMBDA {
		t.Fatalf("want the interpreter's own word for it, got %q", handle.Name())
	}

	if _, err := Wait(_Thread(run), handle); err != nil {
		t.Fatal(err)
	}
}
