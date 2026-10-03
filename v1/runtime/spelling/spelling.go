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
// Nothing here has behaviour beyond Child. It is names, one rule about how an
// id is built from them, and the two mistakes both sides refuse, declared once
// so that errors.Is matches whichever side found one.
package spelling

import (
	"errors"
	"fmt"
)

var (
	// ERR_ARITY is returned for a call passing a function arguments it does not
	// take, whether a compile finds it in a script or a check finds it in a
	// flow.
	ERR_ARITY = errors.New("a call passes arguments the function does not take")

	// ERR_NO_MAIN is returned for a script that defines no entry point, whether
	// it was to be run or read into a flow.
	ERR_NO_MAIN = errors.New("no entry point")

	// ERR_CYCLE is returned for a script whose modules load each other in a
	// ring, whether it was being compiled or read into a flow.
	ERR_CYCLE = errors.New("cycle in the load graph")
)

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

	// SLEEP, REPEAT, RETRY and TIMEOUT are the other builtins a statement is
	// written with. Each reports its line under its own name, as spawn, join
	// and cancel do, and a watcher reads the kind of the line off that name.
	SLEEP   = "sleep"
	REPEAT  = "repeat"
	RETRY   = "retry"
	TIMEOUT = "timeout"

	// IF and MATCH are what a branch's call reports as its builtin: the call
	// that is the whole of an if's branch, or of a match's case or default.
	IF    = "if"
	MATCH = "match"

	// SUBJECT is the local a match evaluates its expression into. A match is
	// an assignment to it followed by a chain comparing it to strings, and a
	// flow does not store it.
	SUBJECT = "_match"

	// CALL is the one function the dialect adds: a statement call of a
	// function the script defines is compiled as a call of this, which
	// reports the line. BUILTIN, BINDING and CALLEE are the keywords the
	// dialect passes on a real call: the if or match a branch's call belongs
	// to, the name a spawn was bound to, and the function a lambda calls.
	//
	// Each starts with a character an identifier cannot, so no script can
	// call CALL, shadow it, or write one of the keywords in a call. A script
	// can still pass one inside a dict it unpacks with **, and all that buys
	// it is a mislabelled line of its own.
	CALL    = "%call"
	BUILTIN = "%builtin"
	BINDING = "%binding"
	CALLEE  = "%callee"
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
