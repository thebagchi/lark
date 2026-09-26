// Package spelling is the vocabulary a script and a graph both name.
//
// The entry point, the thread ids, the builtins that name threads rather than
// values. Four packages used to declare these separately - graph said so in a
// comment, that generating a script should not depend on compiling one - and a
// test compared them pair by pair to catch the drift. The reason was right and
// the remedy was a copy: this package is the reason without the copy, because it
// depends on nothing, not the interpreter and not the compiler, so importing it
// costs neither side anything.
//
// Nothing here has behaviour beyond Child. It is names, and one rule about how
// an id is built from them.
package spelling

import "fmt"

const (
	// ENTRY is the function a run starts at, and the one a graph's spine
	// carries as its first step.
	ENTRY = "main"

	// THREAD prefixes every thread id and HANDLE prefixes a spawned thread's
	// handle. A handle is its thread's id with the one swapped for the other,
	// so thread_1 is h1 and thread_1_1 is h1_1 - unique because the id is, and
	// readable back to the thread it waits for.
	THREAD = "thread_"
	HANDLE = "h"

	// SPINE is the entry point's own thread, the one no spawn started.
	SPINE = THREAD + "0"

	// SPAWN, JOIN and CANCEL are the builtins that name threads rather than
	// values.
	SPAWN  = "spawn"
	JOIN   = "join"
	CANCEL = "cancel"
)

// Child is the id of the ordinal-th thread started by parent.
//
// An id names its parent, so structure can be read off it: thread_1_2 was
// started by thread_1. The spine is the exception, because thread_0_1 would
// carry a 0 that says nothing - every id descends from the spine.
//
// The ordinal is counted per parent rather than per run. One counter shared by
// every thread would number in the order spawns happen, which is a fact about
// time; an id naming its parent is a fact about structure, and the two disagree
// whenever a spawned function spawns before its siblings start.
//
// Revisions:
//   - 2026-09-27 00:10: initial creation, from the copies in scheduler and graph
func Child(parent string, ordinal int) string {
	if parent == SPINE {
		return fmt.Sprintf("%s%d", THREAD, ordinal)
	}

	return fmt.Sprintf("%s_%d", parent, ordinal)
}
