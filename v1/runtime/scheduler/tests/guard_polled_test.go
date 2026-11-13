package tests

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestGuard_PolledNilBuiltin proves Polled takes a typed nil builtin in its
// environment without a panic, and that calling the result gives ErrRun.
//
// Case from Go-level breaker, finding scheduler-F65 (bug B166), reached through
// the exported Polled. A map value is a starlark.Value, so a host can hold a
// nil *starlark.Builtin in it.
//
// Revisions:
//   - 2026-11-06 00:02: initial creation (C100a)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func TestGuard_PolledNilBuiltin(t *testing.T) {
	var (
		nothing *starlark.Builtin
		polled  = scheduler.Polled(starlark.StringDict{
			"x": nothing,
		})
	)

	fn, ok := polled["x"].(*starlark.Builtin)
	if !ok {
		t.Fatalf(
			"polled x is %T, want *starlark.Builtin",
			polled["x"],
		)
	}

	_, err := fn.CallInternal(
		new(starlark.Thread),
		nil,
		nil,
	)
	if !errors.Is(
		err,
		scheduler.ErrRun,
	) {
		t.Fatalf(
			"call gave %v, want ErrRun",
			err,
		)
	}
}
