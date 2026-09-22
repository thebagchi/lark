package args_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/plugin/args"
)

const (
	// FIXTURES is where the scripts these tests run live.
	FIXTURES = "testdata"

	// SUPPLIED is what a caller passes, as a caller writes it.
	SUPPLIED = `{
		"host": "db.internal",
		"port": 5432,
		"ratio": 0.25,
		"tls": true,
		"spare": null,
		"tags": ["a", "b"],
		"labels": {"tier": "gold"},
		"keyword": "supplied"
	}`

	// RUNS is how many runs of one artifact the concurrent test makes.
	RUNS = 8
)

// TestArg_SuppliedWins checks that every shape a run can supply reaches the
// script as the Starlark value it should be, and not as the default.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func TestArg_SuppliedWins(t *testing.T) {
	got := _Ran(t, "declare.star", SUPPLIED)

	want := `["db.internal", 5432, 0.25, True, None, ["a", "b"], {"tier": "gold"}, "supplied"]`
	if got.String() != want {
		t.Fatalf("got %s, want %s", got.String(), want)
	}
}

// TestArg_DefaultWhenNothingSupplied checks that a run supplying nothing gives
// every declaration the value its script stated, including the keyword form.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func TestArg_DefaultWhenNothingSupplied(t *testing.T) {
	got := _Ran(t, "declare.star", "")

	want := `["db.internal", 5432, 0.25, True, None, ["a", "b"], ` +
		`{"tier": "gold"}, "set either way round"]`
	if got.String() != want {
		t.Fatalf("got %s, want %s", got.String(), want)
	}
}

// TestArg_NumbersArriveWhole checks that a JSON number with no fraction
// reaches a script as an int.
//
// JSON has one number type and Starlark has two, so the conversion has to
// choose. A script indexing a list or counting a repeat with a float gets an
// error from Starlark rather than the count its caller passed.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func TestArg_NumbersArriveWhole(t *testing.T) {
	got := _Ran(t, "declare.star", `{"port": 7}`)

	held, ok := got.(*starlark.List)
	if !ok {
		t.Fatalf("returned a %T, want a list", got)
	}

	if held.Index(1).String() != "7" {
		t.Fatalf("port arrived as %s, want 7", held.Index(1).String())
	}
}

// TestArg_RequiredAndMissingFailsTheRun checks that an argument declared with
// no default, and not supplied, stops the run at the declaration.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func TestArg_RequiredAndMissingFailsTheRun(t *testing.T) {
	_, err := _Run(t, "required.star", "{}")
	if !errors.Is(err, args.ErrNotSupplied) {
		t.Fatalf("got %v, want ErrNotSupplied", err)
	}

	got, err := _Run(t, "required.star", `{"token": "t-123"}`)
	if err != nil {
		t.Fatalf("supplied: %v", err)
	}

	if got.String() != `"t-123"` {
		t.Fatalf("got %s, want \"t-123\"", got.String())
	}
}

// TestArg_InABodyIsRefused checks the rule that closes the trap this feature
// would otherwise open.
//
// A body runs on a thread the scheduler made, not the thread a run
// initialises a module on, so no value could reach such a call. It would take
// its default in silence, and deriving a graph would not carry it either.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func TestArg_InABodyIsRefused(t *testing.T) {
	_, err := _Run(t, "inbody.star", `{"host": "db.internal"}`)
	if !errors.Is(err, args.ErrNotDeclaring) {
		t.Fatalf("got %v, want ErrNotDeclaring", err)
	}
}

// TestArg_ALoadedModuleDeclaresItsOwn checks that every unit a run
// initialises is bound, and not only the entry.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func TestArg_ALoadedModuleDeclaresItsOwn(t *testing.T) {
	got := _Ran(t, "module.star", `{"prefix": "redis", "host": "cache.internal"}`)

	if got.String() != `"redis://cache.internal"` {
		t.Fatalf("got %s, want \"redis://cache.internal\"", got.String())
	}
}

// TestArg_TwoRunsOfOneArtifactDiffer is why initialising moved out of the
// compile. One artifact, compiled once, run twice with different arguments.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func TestArg_TwoRunsOfOneArtifactDiffer(t *testing.T) {
	art := _Compiled(t, "module.star")

	first := _Result(t, art, `{"host": "db.internal"}`)
	second := _Result(t, art, `{"prefix": "redis", "host": "cache.internal"}`)

	if first.String() != `"svc://db.internal"` {
		t.Fatalf("first run got %s, want \"svc://db.internal\"", first.String())
	}

	if second.String() != `"redis://cache.internal"` {
		t.Fatalf("second run got %s, want \"redis://cache.internal\"", second.String())
	}
}

// TestArg_ConcurrentRunsKeepTheirOwn checks that runs of one artifact at once
// keep their own arguments and their own globals.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func TestArg_ConcurrentRunsKeepTheirOwn(t *testing.T) {
	art := _Compiled(t, "module.star")

	var group sync.WaitGroup

	for run := range RUNS {
		group.Add(1)

		go func() {
			defer group.Done()

			host := fmt.Sprintf("host-%d", run)

			got := _Result(t, art, fmt.Sprintf(`{"host": %q}`, host))

			want := fmt.Sprintf(`"svc://%s"`, host)
			if got.String() != want {
				t.Errorf("run %d got %s, want %s", run, got.String(), want)
			}
		}()
	}

	group.Wait()
}

// _Ran is what a script returned, failing the test if it did not run.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func _Ran(t *testing.T, name string, supplied string) starlark.Value {
	t.Helper()

	got, err := _Run(t, name, supplied)
	if err != nil {
		t.Fatalf("run %s: %v", name, err)
	}

	return got
}

// _Result is what one run of an artifact returned, failing the test if the
// run did not finish.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func _Result(t *testing.T, art *runtime.Artifact, supplied string) starlark.Value {
	t.Helper()

	got, err := _Invoked(t, art, supplied)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	return got
}

// _Run compiles a fixture and runs it with supplied bound.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func _Run(t *testing.T, name string, supplied string) (starlark.Value, error) {
	t.Helper()

	return _Invoked(t, _Compiled(t, name), supplied)
}

// _Invoked runs an artifact with supplied bound.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func _Invoked(t *testing.T, art *runtime.Artifact, supplied string) (starlark.Value, error) {
	t.Helper()

	ctx := t.Context()

	if supplied != "" {
		parsed, err := runtime.Parsed([]byte(supplied))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}

		ctx = runtime.WithArgs(ctx, parsed)
	}

	return art.Run(ctx)
}

// _Compiled is a fixture compiled, with nothing run.
//
// Revisions:
//   - 2026-09-22 22:56: initial creation
func _Compiled(t *testing.T, name string) *runtime.Artifact {
	t.Helper()

	path := filepath.Join(FIXTURES, name)

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	art, err := runtime.NewCompiler().Compile(path, src)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}

	return art
}

// TestArg_AModuleLevelCheckValidatesWhatWasSupplied is what per-run
// initialising is for, rather than something it costs.
//
// A module-level assert is a script checking its own arguments, and that check
// can only happen once per run because the values do. One artifact takes one
// set of arguments and refuses another, and the refusal is the script's own
// sentence rather than anything this plugin invented.
//
// Revisions:
//   - 2026-09-22 23:34: initial creation
func TestArg_AModuleLevelCheckValidatesWhatWasSupplied(t *testing.T) {
	art := _Compiled(t, "validates.star")

	got := _Result(t, art, `{"port": 5432}`)
	if got.String() != "5432" {
		t.Fatalf("got %s, want 5432", got.String())
	}

	_, err := _Invoked(t, art, `{"port": -1}`)
	if !errors.Is(err, runtime.ErrAssert) {
		t.Fatalf("got %v, want ErrAssert", err)
	}

	// And the same artifact still runs, so the refusal belonged to that run
	// and not to the artifact.
	got = _Result(t, art, `{"port": 80}`)
	if got.String() != "80" {
		t.Fatalf("got %s after the refused run, want 80", got.String())
	}
}
