// Package spec is internal, not public: the fixed words and names of the flow plugin.
//
// Revisions:
//   - 2026-11-08 00:27: initial creation (C91w12a)
package spec

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
const (
	NAME    = "flow"
	ATTEMPT = "n"

	// DELAY is the keyword a repeat or a retry takes its delay by, when the
	// delay is not passed third.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	DELAY = "delay"

	// ONCE is the fewest calls a repeat makes and the fewest attempts a retry
	// may make. WRAPPED is how many arguments every wrapper takes before a
	// delay: how much, and what to do that much of.
	//
	// Revisions:
	//   - 2026-09-20 00:48: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	ONCE    = 1
	WRAPPED = 2
)
