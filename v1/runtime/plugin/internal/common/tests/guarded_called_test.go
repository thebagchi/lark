package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
)

// TestGuarded_CalledNilBuiltin proves Guarded does not panic for a nil inner
// builtin and names the result "builtin".
//
// Case from Q213 (C102), same as C101 for core.
//
// Revisions:
//   - 2026-11-06 21:30: initial creation (C102)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuarded_CalledNilBuiltin(t *testing.T) {
	got := common.Guarded(
		nil,
		nil,
	)
	if got == nil || got.Name() != "builtin" {
		t.Fatalf(
			"got %v, want a builtin named \"builtin\"",
			got,
		)
	}
}
