// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: nothing constructs a Handle until phase 4's spawn, so a test of what a
// handle *is* has to build one directly from its fields. The alternative was to
// ship a constructor this phase has no caller for, which CLAUDE.md's dead-code
// rule forbids. The frozen phase named this cost before the code was written.
package scheduler

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

const (
	WORKER_NAME   = "worker"
	WORKER_THREAD = "thread_2"
	UNHASHABLE    = "unhashable"
)

// _Handle returns a handle as spawn would leave one, without spawning.
//
// Revisions:
//   - 2026-09-19 21:51: initial creation
func _Handle() *Handle {
	return &Handle{
		name:   WORKER_NAME,
		thread: WORKER_THREAD,
		done:   make(chan struct{}),
	}
}

// TestHandle_IsAStarlarkValue proves the type satisfies the interface, at
// compile time rather than by hoping. A type implementing six of the five
// required methods is not a value and cannot be put in a script at all.
//
// Revisions:
//   - 2026-09-19 21:52: initial creation
func TestHandle_IsAStarlarkValue(t *testing.T) {
	var value starlark.Value = _Handle()

	if value.Type() != HANDLE_TYPE {
		t.Fatalf("type is %q, want %q", value.Type(), HANDLE_TYPE)
	}
}

// TestHandle_StringNamesTheFunctionAndTheThread proves what a script sees when
// it prints one, including the thread id a workflow schema records.
//
// Revisions:
//   - 2026-09-19 21:52: initial creation
//   - 2026-09-21 00:59: a thread id is a string that names its parent
func TestHandle_StringNamesTheFunctionAndTheThread(t *testing.T) {
	got := _Handle().String()

	for _, want := range []string{HANDLE_TYPE, WORKER_NAME, "#" + WORKER_THREAD} {
		if !strings.Contains(got, want) {
			t.Fatalf("String is %q, which does not carry %q", got, want)
		}
	}

	t.Logf("a script printing a handle sees: %s", got)
}

// TestHandle_IsAlwaysTrue proves `if h:` tests presence rather than outcome. A
// handle that reported its result through Truth would make a script branch on
// whether a thread had finished, which is join's job.
//
// Revisions:
//   - 2026-09-19 21:53: initial creation
func TestHandle_IsAlwaysTrue(t *testing.T) {
	if _Handle().Truth() != starlark.True {
		t.Fatal("a handle is falsy")
	}
}

// TestHandle_RefusesToHash proves two spawns of one function cannot silently
// collapse into one entry of a set or one key of a dict.
//
// Revisions:
//   - 2026-09-19 21:53: initial creation
func TestHandle_RefusesToHash(t *testing.T) {
	_, err := _Handle().Hash()
	if err == nil {
		t.Fatal("a handle hashed, so two spawns of one function could collide")
	}

	if !strings.Contains(err.Error(), UNHASHABLE) {
		t.Fatalf("refusal does not say why: %v", err)
	}

	t.Logf("refused: %v", err)
}

// TestHandle_FreezeChangesNothing proves freezing is not silently dropping a
// mutation: a handle exposes nothing a script can mutate, so there is nothing
// for freezing to make safe.
//
// Revisions:
//   - 2026-09-19 21:54: initial creation
func TestHandle_FreezeChangesNothing(t *testing.T) {
	handle := _Handle()
	before := handle.String()

	handle.Freeze()

	if handle.String() != before || handle.Name() != WORKER_NAME || handle.Thread() != WORKER_THREAD {
		t.Fatal("freezing a handle changed it")
	}
}

// TestHandle_ReportsItsNameAndThread proves the two accessors a workflow schema
// reads: a thread's index, and which function is running on it.
//
// Revisions:
//   - 2026-09-19 21:54: initial creation
func TestHandle_ReportsItsNameAndThread(t *testing.T) {
	handle := _Handle()

	if handle.Name() != WORKER_NAME {
		t.Fatalf("name is %q, want %q", handle.Name(), WORKER_NAME)
	}

	if handle.Thread() != WORKER_THREAD {
		t.Fatalf("thread is %s, want %s", handle.Thread(), WORKER_THREAD)
	}
}

// TestHandle_HoldsWhatSpawnWillWrite proves the fields a later phase fills are
// present and start empty: value and err, which spawn writes and join reads,
// and stop, which spawn sets and cancel calls.
//
// Nothing here reads value or err after a write by another goroutine, because
// until the done channel closes there is no happens-before edge and such a read
// would be a race the detector will find.
//
// stop is covered here for the same reason as the rest, and it has a second
// effect worth naming: no code in this phase touches that field, so without
// this test the linter reports it as unused. The field is not speculative - the
// frozen phase declares it - but a reader should know the test is the only
// thing referring to it until phase 4.
//
// Revisions:
//   - 2026-09-19 21:55: initial creation
func TestHandle_HoldsWhatSpawnWillWrite(t *testing.T) {
	handle := _Handle()

	if handle.value != nil || handle.err != nil {
		t.Fatal("a fresh handle already carries a result")
	}

	if handle.done == nil {
		t.Fatal("a handle has no channel to publish through")
	}

	if handle.stop != nil {
		t.Fatal("a fresh handle already carries a cancel")
	}

	stopped := false

	handle.stop = func() {
		stopped = true
	}

	handle.stop()

	if !stopped {
		t.Fatal("the cancel a handle carries did not run")
	}

	handle.err = errors.New("scheduler: a spawned failure")

	close(handle.done)
	<-handle.done

	if handle.err == nil {
		t.Fatal("the failure did not survive the publish")
	}
}
