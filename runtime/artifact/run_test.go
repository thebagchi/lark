package artifact_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/artifact"
	"github.com/thebagchi/lark/runtime/scheduler"
)

const (
	CONCURRENT_FIXTURE  = "concurrent.star"
	FORGETS_FIXTURE     = "forgets.star"
	SPINS_FIXTURE       = "spins.star"
	MAINARGS_FIXTURE    = "mainargs.star"
	MAINDEFAULT_FIXTURE = "maindefault.star"
	BREAKS_FIXTURE      = "breaks.star"
	BREAKS_TEXT         = "module level"
	RUNS_OF_ONE         = 2
	EXPECTED_SUM        = 10
	EXPECTED_DEFAULT    = 1
	EXPECTED_DONE       = "done"
	MISSING_GLOBAL      = "absent"
	ENTRY_NAME          = "main"
	ABANDON_BUDGET      = 2 * time.Second
	CANCEL_AFTER        = 20 * time.Millisecond
	CANCEL_BUDGET       = 5 * time.Second
)

// _Built compiles the named fixture or ends the test.
//
// Revisions:
//   - 2026-09-19 22:45: initial creation
func _Built(t *testing.T, name string) *artifact.Artifact {
	t.Helper()

	built, err := artifact.NewCompiler(artifact.WithLoader(_Loader())).
		Compile(name, _Fixture(t, name))
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}

	return built
}

// _Number returns value as an int64 or ends the test.
//
// Revisions:
//   - 2026-09-19 22:45: initial creation
func _Number(t *testing.T, value starlark.Value) int64 {
	t.Helper()

	number, ok := value.(starlark.Int)
	if !ok {
		t.Fatalf("got %T, want starlark.Int", value)
	}

	got, _ := number.Int64()

	return got
}

// TestRun_AScriptCanSpawnAndJoin proves the whole stack from one call: a script
// reaches the scheduler's builtins, spawns two functions that each call into a
// loaded module, and joins them.
//
// This is the first test in which a script - rather than a Go test - uses spawn
// and join at all.
//
// Revisions:
//   - 2026-09-19 22:46: initial creation
func TestRun_AScriptCanSpawnAndJoin(t *testing.T) {
	value, err := _Built(t, CONCURRENT_FIXTURE).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if got := _Number(t, value); got != EXPECTED_SUM {
		t.Fatalf("got %d, want %d", got, EXPECTED_SUM)
	}
}

// TestRun_ReturnsWithoutWaitingForWhatNobodyJoined proves a forgotten spawn
// cannot hold a call open: the run ends when the entry point does.
//
// Revisions:
//   - 2026-09-19 22:47: initial creation
func TestRun_ReturnsWithoutWaitingForWhatNobodyJoined(t *testing.T) {
	start := time.Now()

	value, err := _Built(t, FORGETS_FIXTURE).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	took := time.Since(start)
	if took > ABANDON_BUDGET {
		t.Fatalf("the run waited %s for a thread nobody joined", took)
	}

	text, ok := value.(starlark.String)
	if !ok || string(text) != EXPECTED_DONE {
		t.Fatalf("got %v, want %q", value, EXPECTED_DONE)
	}

	t.Logf("entry point returned and the run ended in %s", took)
}

// TestRun_CancellingTheContextStopsTheSpine proves cancellation reaches the
// entry point's own evaluation, not only spawned ones, and that what comes
// back says so.
//
// The deadline rather than a cancel, so the context's own error is
// DeadlineExceeded - which is the second thing this checks. One sentinel
// covers every stop, and what kind of stop it was stays underneath it.
//
// Revisions:
//   - 2026-09-19 22:48: initial creation
//   - 2026-09-23 07:02: checks which error came back, having only checked that
//     one did
func TestRun_CancellingTheContextStopsTheSpine(t *testing.T) {
	ctx, stop := context.WithTimeout(t.Context(), CANCEL_AFTER)
	defer stop()

	built := _Built(t, SPINS_FIXTURE)

	done := make(chan error, 1)

	go func() {
		_, err := built.Run(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, scheduler.ErrCancelled) {
			t.Fatalf("a cancelled run gave %v, want ErrCancelled", err)
		}

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("a cancelled run gave %v, want it to carry the deadline", err)
		}

		t.Logf("spine stopped: %v", err)
	case <-time.After(CANCEL_BUDGET):
		t.Fatal("a cancelled run did not stop")
	}
}

// TestRun_RefusesAnEntryPointWithArguments proves the refusal names the line
// that defines it, which is why the tree is read rather than the globals.
//
// Revisions:
//   - 2026-09-19 22:49: initial creation
func TestRun_RefusesAnEntryPointWithArguments(t *testing.T) {
	_, err := _Built(t, MAINARGS_FIXTURE).Run(t.Context())
	if !errors.Is(err, artifact.ErrNoMain) {
		t.Fatalf("got %v, want ErrNoMain", err)
	}

	if !strings.Contains(err.Error(), MAINARGS_FIXTURE) {
		t.Fatalf("refusal does not name the position: %v", err)
	}

	t.Logf("refused: %v", err)
}

// TestRun_AcceptsADefaultedEntryPoint proves the count is of parameters a
// caller must supply, not of parameters.
//
// Revisions:
//   - 2026-09-19 22:50: initial creation
func TestRun_AcceptsADefaultedEntryPoint(t *testing.T) {
	value, err := _Built(t, MAINDEFAULT_FIXTURE).Run(t.Context())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if got := _Number(t, value); got != EXPECTED_DEFAULT {
		t.Fatalf("got %d, want %d", got, EXPECTED_DEFAULT)
	}
}

// TestRun_RefusesALibrary proves a module with no entry point compiles and
// cannot be run.
//
// Revisions:
//   - 2026-09-19 22:50: initial creation
func TestRun_RefusesALibrary(t *testing.T) {
	_, err := _Built(t, LIB_FIXTURE).Run(t.Context())
	if !errors.Is(err, artifact.ErrNoMain) {
		t.Fatalf("got %v, want ErrNoMain", err)
	}
}

// TestInvoke_RefusesWhatIsNotACallableGlobal proves the two refusals a host
// tells apart by sentinel rather than by message.
//
// Revisions:
//   - 2026-09-19 22:51: initial creation
func TestInvoke_RefusesWhatIsNotACallableGlobal(t *testing.T) {
	built := _Built(t, CONCURRENT_FIXTURE)

	_, err := built.Invoke(t.Context(), MISSING_GLOBAL)
	if !errors.Is(err, artifact.ErrNoGlobal) {
		t.Fatalf("invoking a missing global gave %v, want ErrNoGlobal", err)
	}
}

// TestInvoke_NumbersEachCallFromItsOwnSpine proves two invocations of one
// artifact are two runs: each numbers its spawns from 1, so a recorded graph
// stays comparable with a later run of the same function.
//
// Revisions:
//   - 2026-09-19 22:52: initial creation
func TestInvoke_NumbersEachCallFromItsOwnSpine(t *testing.T) {
	built := _Built(t, CONCURRENT_FIXTURE)

	for attempt := range 2 {
		value, err := built.Invoke(t.Context(), ENTRY_NAME)
		if err != nil {
			t.Fatalf("invoke %d: %v", attempt, err)
		}

		if got := _Number(t, value); got != EXPECTED_SUM {
			t.Fatalf("invoke %d gave %d, want %d", attempt, got, EXPECTED_SUM)
		}
	}
}

// TestRun_AModuleLevelFailureFailsTheRunNotTheCompile pins where
// initialisation happens.
//
// A script's module-level statements execute once per run, because what they
// produce includes the arguments that run supplied. So a compile of a script
// whose top level fails succeeds, and every run of it fails - which is the
// opposite of what this runtime did until arguments arrived, and is the one
// behaviour that change moved.
//
// Revisions:
//   - 2026-09-22 23:02: initial creation
func TestRun_AModuleLevelFailureFailsTheRunNotTheCompile(t *testing.T) {
	built, err := artifact.NewCompiler(artifact.WithLoader(_Loader())).
		Compile(BREAKS_FIXTURE, _Fixture(t, BREAKS_FIXTURE))
	if err != nil {
		t.Fatalf("compile %s: %v", BREAKS_FIXTURE, err)
	}

	for attempt := range RUNS_OF_ONE {
		_, err = built.Run(context.Background())
		if err == nil {
			t.Fatalf("run %d succeeded, want the module-level failure", attempt)
		}

		if !strings.Contains(err.Error(), BREAKS_TEXT) {
			t.Fatalf("run %d failed with %v, want it to name %q", attempt, err, BREAKS_TEXT)
		}
	}
}
