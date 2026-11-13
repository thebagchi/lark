// This file tests Checked with a nil builtin, which no script can build, because
// the wrapper of NewBuiltin always passes its own builtin.
//
// Revisions:
//   - 2026-11-06 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:16: moved to package registry (C91w1)
//   - 2026-11-10 00:25: moved to tests/ from guard_called_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/internal/registry"
)

// TestGuard_CalledNilBuiltin proves Checked does not panic for a nil builtin
// and names the result "builtin".
//
// Case from Q213 (C102), same as C101 for core.
//
// Revisions:
//   - 2026-11-06 21:30: initial creation (C102)
//   - 2026-11-07 18:16: moved to package registry (C91w1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_CalledNilBuiltin(t *testing.T) {
	got := registry.Checked(nil)
	if got == nil || got.Name() != "builtin" {
		t.Fatalf(
			"got %v, want a builtin named \"builtin\"",
			got,
		)
	}
}
