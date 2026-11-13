// Package spec is internal, not public: the fixed words and names of the args plugin.
//
// Revisions:
//   - 2026-11-08 00:27: initial creation (C91w12a)
package spec

const (
	// NAME is what this plugin is called when a conflict has to name it.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	NAME = "args"

	// DEFAULT is the second parameter, so a script may pass it either way
	// round.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	DEFAULT = "default"

	// SUPPLIED is the first, which is the name a run supplies it under rather
	// than the name the script binds.
	//
	// Revisions:
	//   - 2026-09-22 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	SUPPLIED = "name"
)
