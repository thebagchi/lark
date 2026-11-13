package common

// MAX_DEPTH is the longest chain of loads a compile or a derivation
// follows before it refuses the script.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
const (
	MAX_DEPTH = 256
)

// ENTRY is the function a run starts at, and the one a graph's spine
// carries as its first step.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
const (
	ENTRY = "main"

	// THREAD prefixes every thread id and HANDLE prefixes a spawned thread's
	// handle. A handle is its thread's id with the one swapped for the other,
	// so thread_1 is h1 and thread_1_1 is h1_1 - unique because the id is, and
	// readable back to the thread it waits for.
	//
	// Revisions:
	//   - 2026-09-21 01:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
	THREAD = "thread_"

	// SPINE is the entry point's own thread, the one no spawn started.
	//
	// Revisions:
	//   - 2026-09-21 01:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
	SPINE = THREAD + "0"

	// SPAWN, JOIN and CANCEL are the builtins that name threads rather than
	// values.
	//
	// Revisions:
	//   - 2026-09-19 23:36: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
	SPAWN  = "spawn"
	JOIN   = "join"
	CANCEL = "cancel"

	// SLEEP, REPEAT, RETRY and TIMEOUT are the other builtins a statement is
	// written with. Each reports its line under its own name, as spawn, join
	// and cancel do, and a watcher reads the kind of the line off that name.
	//
	// Revisions:
	//   - 2026-09-20 21:20: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
	SLEEP   = "sleep"
	REPEAT  = "repeat"
	RETRY   = "retry"
	TIMEOUT = "timeout"

	// IF and MATCH are what a branch's call reports as its builtin: the call
	// that is the whole of an if's branch, or of a match's case or default.
	//
	// Revisions:
	//   - 2026-09-20 21:20: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
	IF    = "if"
	MATCH = "match"

	// SUBJECT is the local a match evaluates its expression into. A match is
	// an assignment to it followed by a chain comparing it to strings, and a
	// flow does not store it.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
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
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:52: moved here from spelling (Q244, C91w12d)
	CALL    = "%call"
	BUILTIN = "%builtin"
	BINDING = "%binding"
	CALLEE  = "%callee"
)
