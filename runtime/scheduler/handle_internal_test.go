// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: a test of what a handle *is* builds one directly from its fields, so
// that what a script sees is checked apart from what a spawn does.
package scheduler

import (
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
// compile time rather than by hoping.
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
}

// TestHandle_FreezeChangesNothing proves freezing is not silently dropping a
// mutation: a handle exposes nothing a script can mutate.
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
