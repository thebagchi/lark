//go:build amd64 && !purego

package spec

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// LEVELS are the levels that have kernels, the widest first.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
var (
	LEVELS = [...]cpu.Level{
		cpu.LEVEL_AVX512,
		cpu.LEVEL_AVX2,
		cpu.LEVEL_SSE,
	}
)
