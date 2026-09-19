package artifact_test

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

const (
	EXPLODES_FIXTURE = "explodes.star"
	PANIC_TEXT       = "a plugin blew up"
	RECOVERED        = "recovered"
)

// _Exploding is a plugin whose one builtin panics.
type _Exploding struct{}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-09-19 23:22: initial creation
func (e *_Exploding) Name() string {
	return "exploding"
}

// Values returns a builtin that panics when a script calls it.
//
// Revisions:
//   - 2026-09-19 23:22: initial creation
func (e *_Exploding) Values() starlark.StringDict {
	return starlark.StringDict{
		"explode": starlark.NewBuiltin("explode", func(
			thread *starlark.Thread,
			fn *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			panic(PANIC_TEXT)
		}),
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
func TestRun_APanickingBuiltinFailsTheRunNotTheProcess(t *testing.T) {
	_, err := _Built(t, EXPLODES_FIXTURE).Run(t.Context())
	if err == nil {
		t.Fatal("a panicking builtin did not fail the run")
	}

	if !strings.Contains(err.Error(), PANIC_TEXT) {
		t.Fatalf("the failure does not carry what the plugin panicked with: %v", err)
	}

	if !strings.Contains(err.Error(), RECOVERED) {
		t.Fatalf("the failure does not say it was recovered: %v", err)
	}

	t.Logf("the host survived: %v", err)
}
