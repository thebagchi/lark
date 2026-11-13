package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// TestDetect_AgreesWithTheLevelFoundAtStart proves Detect is stable, and that
// LEVEL is one of the four levels. On a build with no kernels it is the lowest.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestDetect_AgreesWithTheLevelFoundAtStart(t *testing.T) {
	if cpu.Detect() != cpu.LEVEL {
		t.Fatalf(
			"Detect %d, LEVEL %d",
			cpu.Detect(),
			cpu.LEVEL,
		)
	}

	if cpu.LEVEL < cpu.LEVEL_SWAR || cpu.LEVEL > cpu.LEVEL_AVX512 {
		t.Fatalf(
			"LEVEL %d is not a level",
			cpu.LEVEL,
		)
	}

	t.Logf(
		"level %d",
		cpu.LEVEL,
	)
}
