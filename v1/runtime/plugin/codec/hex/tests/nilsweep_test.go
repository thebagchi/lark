package tests

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/nilsweep"
)

// TestNilSweep_hex calls the plugin's methods on a nil and a zero receiver, and
// every builtin of its module with each nil or odd argument shape a host or a
// plugin author could pass (C87, Q2, Q85). A panic or a hang fails the row.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestNilSweep_hex(t *testing.T) {
	t.Run(
		"nil and zero receivers",
		func(t *testing.T) {
			nilsweep.Methods(
				t,
				"hex",
			)
		},
	)

	members := nilsweep.Members(
		t,
		"hex",
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

	nilsweep.Rows(
		t,
		[]nilsweep.Row{
			{
				Name: "Encode nil data",
				Call: func() error {
					if hex.Encode(nil) != "" {
						return errors.New("text")
					}

					return nil
				},
			},
			{
				Name: "Encode empty data",
				Call: func() error {
					if hex.Encode([]byte{}) != "" {
						return errors.New("text")
					}

					return nil
				},
			},
		},
	)
}
