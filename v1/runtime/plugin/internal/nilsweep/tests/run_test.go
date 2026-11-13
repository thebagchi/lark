package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/internal/nilsweep"
)

// TestRun_NamesANilBuiltin proves Run reports a nil builtin and does not
// panic while it names it. Run fails its t for every shape, because a nil
// builtin cannot be called; the inner run is a test of its own, so its
// failure does not fail this one. The name in the failure text is "builtin".
//
// Case from the C102 test round 1: nilsweep.go had no test.
//
// Revisions:
//   - 2026-11-06 22:30: initial creation (C102 fix round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRun_NamesANilBuiltin(t *testing.T) {
	var (
		inner = []testing.InternalTest{
			{
				Name: "NilBuiltin",
				F: func(t *testing.T) {
					t.Helper()

					nilsweep.Run(
						t,
						nil,
					)
				},
			},
		}
		ok = testing.RunTests(
			func(
				string,
				string,
			) (bool, error) {
				return true, nil
			},
			inner,
		)
	)

	if ok {
		t.Fatal("got a pass, want the inner run to report the nil builtin")
	}
}
