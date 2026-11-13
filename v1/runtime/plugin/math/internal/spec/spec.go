// Package spec is internal, not public: the fixed words and constants of the math plugin.
//
// Revisions:
//   - 2026-11-08 00:27: initial creation (C91w12a)
package spec

import (
	"math"
)

const (
	// NAME is the module.
	//
	// Revisions:
	//   - 2026-09-20 00:24: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	NAME = "math"

	// DEGREES is how many a turn has, which the two conversions share.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	DEGREES = 360

	// TURN is a full circle in radians, which is what tau names for a script.
	//
	// Revisions:
	//   - 2026-09-26 23:55: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	TURN = 2 * math.Pi
)
