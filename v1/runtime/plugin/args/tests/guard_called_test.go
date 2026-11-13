package tests

// This file tests that the helpers of the args plugin that name their builtin
// in an error do not panic when the builtin is nil. No script can build that
// input, because the wrapper of NewBuiltin always passes its own builtin, so
// the tests call the helpers directly (C102, Q213).
//
// Revisions:
//   - 2026-11-06 22:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from guard_called_internal_test.go and rewrote its
//     header (C120b, Q311, Q322)

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/args/internal/declare"
)

// TestGuard_CalledNilBuiltin proves each helper that names its builtin in an
// error does not panic when the builtin is nil.
//
// Case from Q213 (C102), same as C101 for core.
//
// Revisions:
//   - 2026-11-06 21:28: initial creation (C102)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_CalledNilBuiltin(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			"_Declare",
			func() error {
				_, err := declare.Declare(
					new(starlark.Thread),
					nil,
					nil,
					nil,
				)

				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				defer func() {
					if raised := recover(); raised != nil {
						t.Fatalf(
							"panicked: %v",
							raised,
						)
					}
				}()

				_ = test.call()
			},
		)
	}
}
