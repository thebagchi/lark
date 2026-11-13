// Package spec is internal, not public: the constants spelling uses and no other package reads.
//
// Revisions:
//   - 2026-11-08 00:52: initial creation, from spelling (Q243, C91w12d)
package spec

// DIGITS is the room Child keeps on the stack for an ordinal: the 19 digits
// of the widest int and its sign. DECIMAL is the base an ordinal is written in.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: moved here from spelling (Q243, C91w12d)
const (
	DIGITS  = 20
	DECIMAL = 10
)

// THREAD prefixes every thread id and HANDLE prefixes a spawned thread's
// handle. A handle is its thread's id with the one swapped for the other,
// so thread_1 is h1 and thread_1_1 is h1_1 - unique because the id is, and
// readable back to the thread it waits for.
//
// Revisions:
//   - 2026-09-21 01:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: moved here from spelling (Q243, C91w12d)
const (
	HANDLE = "h"
)

// MAX_WORD is the longest word Nearest compares. A longer word is not a typo
// of a name, and the compare costs the product of the two lengths.
//
// Revisions:
//   - 2026-11-05 16:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: moved here from spelling (Q243, C91w12d)
const (
	MAX_WORD = 64
)
