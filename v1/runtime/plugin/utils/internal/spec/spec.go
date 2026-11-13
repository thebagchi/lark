// Package spec is internal, not public: the fixed words and layout of the utils plugin.
//
// Revisions:
//   - 2026-11-08 00:27: initial creation (C91w12a)
package spec

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
const (
	NAME     = "utils"
	DATETIME = "datetime"

	// LAYOUT is the brief's YYYY-MM-DD HH:MM:SS.uuuuuu, in the reference time
	// Go writes a layout as.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	LAYOUT = "2006-01-02 15:04:05.000000"
)
