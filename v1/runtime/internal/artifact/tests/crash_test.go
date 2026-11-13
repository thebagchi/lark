package tests

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

const (
	// EXPLODES_FIXTURE is a script whose main joins a spawned thread that calls a builtin that
	// panics.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/explodes.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	EXPLODES_FIXTURE = `
def boom():
    explode()
    return 1

def main():
    return join(spawn(boom))
`

	// UNJOINED_FIXTURE is a script whose main spawns a thread that calls a builtin that
	// panics, and never joins it.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/unjoined.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	UNJOINED_FIXTURE = `
def boom():
    explode()
    return 1

def main():
    spawn(boom)
    sleep(30000)
    return "ok"
`
)

const (
	PANIC_TEXT = "a plugin blew up"
	RECOVERED  = "recovered"
)

// _Exploding is a plugin whose one builtin panics.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Exploding struct{}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-09-19 23:22: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (e *_Exploding) Name() string {
	return "exploding"
}

// Values returns a builtin that panics when a script calls it.
//
// Revisions:
//   - 2026-09-19 23:22: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (e *_Exploding) Values() starlark.StringDict {
	return starlark.StringDict{
		"explode": starlark.NewBuiltin(
			"explode",
			func(
				thread *starlark.Thread,
				fn *starlark.Builtin,
				args starlark.Tuple,
				kwargs []starlark.Tuple,
			) (starlark.Value, error) {
				panic(PANIC_TEXT)
			},
		),
	}
}

// TestRun_APanickingBuiltinFailsTheRunNotTheProcess proves a script cannot
// crash its host.
//
// The builtin it reaches panics inside a spawned goroutine, where a panic
// cannot be recovered from outside. Without a guard this test does not fail -
// it kills the test binary, and go test reports no failing test at all.
//
// Revisions:
//   - 2026-09-19 23:23: initial creation
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRun_APanickingBuiltinFailsTheRunNotTheProcess(t *testing.T) {
	_, err := artifact.Run(
		t.Context(),
		_Built(
			t,
			"explodes.star",
		),
	)
	if err == nil {
		t.Fatal("a panicking builtin did not fail the run")
	}

	if !strings.Contains(
		err.Error(),
		PANIC_TEXT,
	) {
		t.Fatalf(
			"the failure does not carry what the plugin panicked with: %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		RECOVERED,
	) {
		t.Fatalf(
			"the failure does not say it was recovered: %v",
			err,
		)
	}

	t.Logf(
		"the host survived: %v",
		err,
	)
}

// TestRegression_Panic proves a panic in a spawn that nobody joins still ends
// the run. The script spawns boom, does not join it, and sleeps for thirty
// seconds before it returns "ok". Without the fix the run returns "ok" with no
// error, because nothing calls _End.
//
// The sleep keeps main alive until boom panics. A main that returns at once
// ends the run, and that cancel can reach boom before its panic, so the test
// fails about one time in a thousand. The panic cancels the sleep, so the run
// ends at once.
//
// Revisions:
//   - 2026-10-24 20:00: initial creation
//   - 2026-10-28 18:45: main sleeps after the spawn, so the end of main cannot
//     cancel boom before its panic
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRegression_Panic(t *testing.T) {
	_, err := artifact.Run(
		t.Context(),
		_Built(
			t,
			"unjoined.star",
		),
	)
	if err == nil {
		t.Fatal("an unjoined panic did not fail the run")
	}

	if !strings.Contains(
		err.Error(),
		PANIC_TEXT,
	) {
		t.Fatalf(
			"the failure does not carry what the plugin panicked with: %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		RECOVERED,
	) {
		t.Fatalf(
			"the failure does not say it was recovered: %v",
			err,
		)
	}

	t.Logf(
		"unjoined panic ended the run: %v",
		err,
	)
}
