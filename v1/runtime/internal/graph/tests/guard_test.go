package tests

// This file tests the guards of a reading method, a scope method and two placing
// functions against a nil that no exported function passes (C100c, Q206; breaker
// findings graph-F4 to F9, F11, F13, F15, F17 to F19).
//
// Revisions:
//   - 2026-11-10 00:25: moved to tests/ from guard_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)

import (
	"errors"
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestGuards_NilLoadIsErrModule checks a nil load statement is refused with
// ErrUnresolved, as a load that names no module is.
//
// Revisions:
//   - 2026-11-06 22:02: initial creation (C100c, B180)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuards_NilLoadIsErrModule(t *testing.T) {
	loads := map[string]*syntax.LoadStmt{
		"nil load":        nil,
		"load, no module": {},
	}

	for name, load := range loads {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := new(graph.Reading).Loads(
					load,
					"",
				)
				if !errors.Is(
					err,
					graph.ErrUnresolved,
				) {
					t.Fatalf(
						"got %v, want ErrUnresolved",
						err,
					)
				}
			},
		)
	}
}

// TestGuards_EmptyBlockPlacesAbove checks a comment before an if with no
// block is placed above the clause, and a nil above is no panic.
//
// Revisions:
//   - 2026-11-06 22:02: initial creation (C100c, B154)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuards_EmptyBlockPlacesAbove(t *testing.T) {
	for name, node := range map[string]*syntax.IfStmt{
		"empty block": {},
		"nil if":      nil,
	} {
		t.Run(
			name,
			func(t *testing.T) {
				called := 0

				graph.PlaceClause(
					nil,
					node,
					syntax.Comment{},
					func() {
						called++
					},
				)

				if called != 1 {
					t.Fatalf(
						"above called %d times, want 1",
						called,
					)
				}
			},
		)
	}

	graph.PlaceClause(
		nil,
		&syntax.IfStmt{},
		syntax.Comment{},
		nil,
	)
}
