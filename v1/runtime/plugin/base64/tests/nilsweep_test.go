package tests

import (
	"maps"
	"slices"
	"testing"

	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/nilsweep"
)

// TestNilSweep_base64 calls the plugin's methods on a nil and a zero receiver, and
// every builtin of its module with each nil or odd argument shape a host or a
// plugin author could pass (C87, Q2, Q85). A panic or a hang fails the row.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestNilSweep_base64(t *testing.T) {
	t.Run(
		"nil and zero receivers",
		func(t *testing.T) {
			nilsweep.Methods(
				t,
				"base64",
			)
		},
	)

	members := nilsweep.Members(
		t,
		"base64",
	)

	for _, name := range slices.Sorted(maps.Keys(members)) {
		t.Run(
			name,
			func(t *testing.T) {
				nilsweep.Run(
					t,
					members[name],
				)
			},
		)
	}
}
