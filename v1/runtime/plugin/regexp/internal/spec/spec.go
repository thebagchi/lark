// Package spec is internal, not public: the names the regexp module spells, the
// match bounds and the cache size.
//
// Revisions:
//   - 2026-11-08 00:26: initial creation (C91w12b)
package spec

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:26: moved here from plugin/regexp (C91w12b)
const (
	NAME    = "regexp"
	MATCH   = "match"
	SEARCH  = "search"
	FINDALL = "findall"
	SUB     = "sub"
	SPLIT   = "split"
	QUOTE   = "quote"

	// BOUNDS is how many numbers FindSubmatchIndex gives per group: where it
	// started and where it ended. So the group count is the slice over this.
	//
	// Revisions:
	//   - 2026-09-26 23:55: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:26: moved here from plugin/regexp (C91w12b)
	BOUNDS = 2

	// TEXT, START, END, GROUPS and NAMED are what a match calls its parts.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:26: moved here from plugin/regexp (C91w12b)
	TEXT   = "text"
	START  = "start"
	END    = "end"
	GROUPS = "groups"
	NAMED  = "named"

	// COUNT is the optional limit sub and split take, and ALL is what it
	// means when nobody gives one.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:26: moved here from plugin/regexp (C91w12b)
	COUNT = "count"
	ALL   = 0

	// KEPT is how many compiled patterns are held before the lot is dropped.
	//
	// A cache keyed by something a script writes is unbounded unless somebody
	// bounds it, and a workflow that builds patterns in a loop would grow it
	// forever. Dropping all of them rather than the oldest is deliberate: what
	// this buys is not compiling the same pattern twice in a loop, and a loop
	// re-fills the cache in one pass.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:26: moved here from plugin/regexp (C91w12b)
	KEPT = 256
)
