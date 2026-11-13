// Package spec is internal, not public: the fixed words and names of the event plugin.
//
// Revisions:
//   - 2026-11-08 00:27: initial creation (C91w12a)
package spec

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-30 21:12: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
const (
	NAME = "event"
	POST = "post"
	WAIT = "wait"

	// EXPIRED is what a wait that ran out puts in the second half of its
	// answer. A message rather than a bool, because it is what a script prints
	// or fails with.
	//
	// Revisions:
	//   - 2026-09-30 21:12: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	EXPIRED = "timed out"
)

const (
	// EVENTS is the name a run keeps its events under.
	//
	// Revisions:
	//   - 2026-09-30 21:12: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	EVENTS = "event.latches"
)
