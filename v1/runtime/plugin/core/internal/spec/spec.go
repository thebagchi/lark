// Package spec is internal, not public: the names the core builtins and their
// outcome records spell, and two hints.
//
// Revisions:
//   - 2026-11-08 00:26: initial creation (C91w12b)
package spec

// NAME is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:26: moved here from plugin/core (C91w12b)
const (
	NAME = "core"

	ASSERT = "assert"

	OUTCOMES = "outcomes"

	// STR, REPR, PRINT and FAIL are the library builtins of those names,
	// guarded against a value nested past common.DEPTH (C47).
	//
	// Revisions:
	//   - 2026-11-03 21:51: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:26: moved here from plugin/core (C91w12b)
	STR   = "str"
	REPR  = "repr"
	PRINT = "print"
	FAIL  = "fail"
)

// CANCEL_HINT is the fix that ends the error of a join of a thread that was
// cancelled.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
//   - 2026-11-08 00:26: moved here from plugin/core (C91w12b)
const (
	CANCEL_HINT = "read a cancelled thread with outcomes()"
)

// The words a record's "kind" holds. A script compares against these, so they
// do not follow the Go names of the errors they stand for.
//
// Revisions:
//   - 2026-11-02 17:03: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:26: moved here from plugin/core (C91w12b)
const (
	KIND_FAIL      = "fail"
	KIND_CANCELLED = "cancelled"
	KIND_CYCLE     = "cycle"
	KIND_NESTED    = "nested"
)

// CAPTURE_HINT is the fix that ends the text of core.ErrClosure: a lambda written
// inside spawn takes its variables at the spawn.
//
// Convention (Q158): a hint follows the failing text as `<text> (<hint>)`, in
// lower case.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
//   - 2026-11-08 00:26: moved here from plugin/core (C91w12b)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 17:15: uses ErrClosure, formerly ErrCaptures (C121, Q348)
const (
	CAPTURE_HINT = "write spawn(lambda: f(x)) to give the thread x as it is " +
		"at the spawn"
)
