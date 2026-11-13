package tests

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestFork_RefusesAThreadThatLoadsAModule proves Fork is refused with
// script.ErrInit when the thread carries the mark of a module's top level, and
// that the text names no builtin when none is running on it (B278). The tests
// of the runtime prove the same call from a builtin of the host.
//
// Revisions:
//   - 2026-11-12 17:20: initial creation (C128, B278, Q389)
//   - 2026-11-12 18:58: renamed from TestBeside_RefusesAThreadThatLoadsAModule (C129, Q396)
//   - 2026-11-13 12:57: the mark is a map of workflow.Value (C132, Q437)
func TestFork_RefusesAThreadThatLoadsAModule(t *testing.T) {
	thread := new(starlark.Thread)

	thread.SetLocal(
		script.ARGS_LOCAL,
		nil,
	)

	if script.Loads(thread) {
		t.Fatal("a nil local is not the mark")
	}

	_, err := scheduler.Fork(
		thread,
		nil,
	)
	if errors.Is(
		err,
		script.ErrInit,
	) {
		t.Fatalf(
			"an unmarked thread: got %v, want anything but ErrInit",
			err,
		)
	}

	thread.SetLocal(
		script.ARGS_LOCAL,
		map[string]*workflowpb.Value(nil),
	)

	_, err = scheduler.Fork(
		thread,
		nil,
	)
	if !errors.Is(
		err,
		script.ErrInit,
	) {
		t.Fatalf(
			"a marked thread: got %v, want ErrInit",
			err,
		)
	}

	want := "cannot start a thread while the module loads " +
		"(call it from main or a function main calls)"
	if err.Error() != want {
		t.Fatalf(
			"got %q, want %q",
			err.Error(),
			want,
		)
	}
}
