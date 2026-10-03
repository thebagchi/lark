package artifact_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/script"
)

// ERR_SPELLING is what the plugin below refuses with, so that a test can tell
// being asked from not being asked.
var ERR_SPELLING = errors.New("refused on spelling")

const (
	CONCURRENT_FIXTURE  = "concurrent.star"
	FORGETS_FIXTURE     = "forgets.star"
	SPINS_FIXTURE       = "spins.star"
	MAINARGS_FIXTURE    = "mainargs.star"
	MAINDEFAULT_FIXTURE = "maindefault.star"
	BREAKS_FIXTURE      = "breaks.star"
	BREAKS_TEXT         = "module level"
	RUNS_OF_ONE         = 2

	// SPELLING is the one name the plugin below supplies, and the name a file
	// may take for itself.
	SPELLING         = "spelling"
	EXPECTED_SUM     = 10
	EXPECTED_DEFAULT = 1
	EXPECTED_DONE    = "done"
	ABANDON_BUDGET   = 2 * time.Second
	CANCEL_AFTER     = 20 * time.Millisecond
	CANCEL_BUDGET    = 5 * time.Second
)

// _Built compiles the named fixture or ends the test.
//
// Revisions:
//   - 2026-09-19 22:45: initial creation
func _Built(t *testing.T, name string) *artifact.Artifact {
	t.Helper()

	built, err := artifact.Compile(&script.Source{
		Entry:  name,
		Text:   _Fixture(t, name),
		Loader: _Loader(),
	})
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
	value, err := artifact.Run(t.Context(), _Built(t, CONCURRENT_FIXTURE))
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

	value, err := artifact.Run(t.Context(), _Built(t, FORGETS_FIXTURE))
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
		_, err := artifact.Run(ctx, built)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, scheduler.ERR_CANCELLED) {
			t.Fatalf("a cancelled run gave %v, want ERR_CANCELLED", err)
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
	_, err := artifact.Run(t.Context(), _Built(t, MAINARGS_FIXTURE))
	if !errors.Is(err, artifact.ERR_NO_MAIN) {
		t.Fatalf("got %v, want ERR_NO_MAIN", err)
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
	value, err := artifact.Run(t.Context(), _Built(t, MAINDEFAULT_FIXTURE))
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
	_, err := artifact.Run(t.Context(), _Built(t, LIB_FIXTURE))
	if !errors.Is(err, artifact.ERR_NO_MAIN) {
		t.Fatalf("got %v, want ERR_NO_MAIN", err)
	}
}

// TestRun_NumbersEachRunFromItsOwnSpine proves two runs of one artifact are
// two runs: each numbers its spawns from 1, so a recorded graph stays
// comparable with a later run of the same script.
//
// Revisions:
//   - 2026-09-19 22:52: initial creation, as TestInvoke_NumbersEachCallFromItsOwnSpine
//   - 2026-10-03 00:21: runs the entry point twice, a run calling it and nothing else
func TestRun_NumbersEachRunFromItsOwnSpine(t *testing.T) {
	built := _Built(t, CONCURRENT_FIXTURE)

	for attempt := range 2 {
		value, err := artifact.Run(t.Context(), built)
		if err != nil {
			t.Fatalf("run %d: %v", attempt, err)
		}

		if got := _Number(t, value); got != EXPECTED_SUM {
			t.Fatalf("run %d gave %d, want %d", attempt, got, EXPECTED_SUM)
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
	built, err := artifact.Compile(&script.Source{
		Entry:  BREAKS_FIXTURE,
		Text:   _Fixture(t, BREAKS_FIXTURE),
		Loader: _Loader(),
	})
	if err != nil {
		t.Fatalf("compile %s: %v", BREAKS_FIXTURE, err)
	}

	for attempt := range RUNS_OF_ONE {
		_, err = artifact.Run(context.Background(), built)
		if err == nil {
			t.Fatalf("run %d succeeded, want the module-level failure", attempt)
		}

		if !strings.Contains(err.Error(), BREAKS_TEXT) {
			t.Fatalf("run %d failed with %v, want it to name %q",
				attempt, err, BREAKS_TEXT)
		}
	}
}

// _Spelling is a plugin that refuses every call of the one name it supplies,
// which is how a check that matches on spelling behaves when the spelling is
// not its own.
type _Spelling struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 20:46: initial creation
func (s *_Spelling) Name() string {
	return SPELLING
}

// Values supplies the one name.
//
// Revisions:
//   - 2026-09-24 20:46: initial creation
func (s *_Spelling) Values() starlark.StringDict {
	return starlark.StringDict{
		SPELLING: starlark.NewBuiltin(SPELLING, func(
			thread *starlark.Thread,
			fn *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			return starlark.None, nil
		}),
	}
}

// Check refuses any file at all, so that the only thing deciding the outcome
// is whether it was asked.
//
// Revisions:
//   - 2026-09-24 20:46: initial creation
func (s *_Spelling) Check(tree *syntax.File) error {
	return ERR_SPELLING
}

// TestCheck_APluginIsNotAskedAboutANameTheFileHasTaken is the mistake the
// compiler now carries so a plugin cannot make it.
//
// A global shadows a predeclared one. A check that matches on the spelling a
// plugin owns, run against a file that means something else by that name,
// refuses a script that runs perfectly well and blames a plugin it never
// reached. That was found twice in one day in two plugins, so the question is
// asked here rather than by each of them.
//
// Revisions:
//   - 2026-09-24 20:46: initial creation
//   - 2026-10-02 13:12: hands the compiler the plugin, there being no registry
//     to build
func TestCheck_APluginIsNotAskedAboutANameTheFileHasTaken(t *testing.T) {
	spelling := new(_Spelling)

	plugins := artifact.WithPlugins(spelling)

	// This file leaves the name alone, so the plugin is asked and refuses.
	uses := []byte("def main():\n    " + SPELLING + "()\n")

	_, err := artifact.Compile(&script.Source{Entry: "uses.star", Text: uses}, plugins)
	if !errors.Is(err, ERR_SPELLING) {
		t.Fatalf("a file using the name gave %v, want the plugin to have been asked", err)
	}

	// And every way a file can take it, because missing one is the whole
	// mistake rather than a near miss: the name then looks like the plugin's,
	// and a correct script is refused and told about a plugin it never
	// reached.
	taken := map[string]string{
		"an assignment":        SPELLING + " = 1",
		"a def":                "def " + SPELLING + "():\n    return 1",
		"a tuple":              SPELLING + ", other = 1, 2",
		"a list":               "[" + SPELLING + ", other] = [1, 2]",
		"a loop variable":      "for " + SPELLING + " in [1]:\n    pass",
		"under a top-level if": "if True:\n    " + SPELLING + " = 1",
	}

	for name, binding := range taken {
		t.Run(name, func(t *testing.T) {
			src := []byte(binding + "\n\ndef main():\n    return 1\n")

			_, err := artifact.Compile(
				&script.Source{Entry: name + ".star", Text: src},
				plugins,
			)
			if err != nil {
				t.Fatalf("a file that took the name by %s was refused: %v",
					name, err)
			}
		})
	}

	// A load binds too, and needs a loader to reach the file it names.
	t.Run("a load", func(t *testing.T) {
		src := `load("spelling.star", "` + SPELLING + `")` +
			"\n\ndef main():\n    return " + SPELLING + "\n"

		_, err := artifact.Compile(
			&script.Source{
				Entry:  "loads.star",
				Text:   []byte(src),
				Loader: _Loader(),
			},
			plugins,
		)
		if err != nil {
			t.Fatalf("a file that took the name by a load was refused: %v", err)
		}
	})
}
