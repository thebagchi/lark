package run

import (
	"math"
)

// ABSENT is the index slices.IndexFunc answers when nothing matches.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	ABSENT = -1
)

// PAIR is the length of a keyword's tuple: its name, then its value.
//
// Revisions:
//   - 2026-11-03 06:44: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	PAIR = 2
)

// RELEASING is the value of Locals.HoldNumber while a Release clears the hold.
// A hold is numbered from 1 up, so the largest value is never a hold's number.
//
// Revisions:
//   - 2026-11-10 12:15: initial creation (C116 fix round 1, B236, Q329)
const (
	RELEASING = math.MaxUint64
)

// STOPPED is what the interpreter is told when a run is stopped, and what it
// then puts in the error it raises.
//
// A fixed word rather than the context's own text. Passing ctx.Err().Error()
// here put "context canceled" in the interpreter's message, and the caller
// wraps the context's error as well so that it stays matchable - so the same
// three words arrived twice in one sentence.
//
// Revisions:
//   - 2026-09-23 22:46: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	STOPPED = "the run was stopped"
)

// MAX_MILLIS is the longest duration a script may ask for, in milliseconds:
// what the schema's int32 can hold, about 24.8 days.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	MAX_MILLIS = math.MaxInt32
)

// HANDLE_TYPE is what a script's type() reports for a handle.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	HANDLE_TYPE = "handle"
)

// BUILTIN_FILE is the file name the interpreter gives the frame of a builtin,
// which has no source. A frame with this name, or with no line, has no place
// to name.
//
// Revisions:
//   - 2026-11-04 18:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	BUILTIN_FILE = "<builtin>"
)

// CALLER is how deep a builtin's caller sits on the interpreter's call
// stack: depth 0 is the builtin itself.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	CALLER = 1
)

// LAMBDA is what the interpreter calls an anonymous function. Nothing refuses
// one; this is here so a reporter can tell that a name is not one.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	LAMBDA = "lambda"
)

// LOCALS_KEY names the one thing a run leaves on an interpreter thread. A
// Starlark builtin is handed only a thread, so this is how every builtin
// here finds the evaluation it belongs to.
//
// One local rather than several, so a child inherits everything by copying
// a struct and nothing is inherited by accident or dropped by omission.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
const (
	LOCALS_KEY = "scheduler.locals"

	// FIRST_SPAWN is the ordinal the first child of any thread takes. Thread
	// ids use common.SPINE, the entry point's id, and common.THREAD, the
	// prefix of every id.
	//
	// An id names its parent: the spine's children are thread_1 and thread_2,
	// and thread_1's own first child is thread_1_1. The spine contributes no
	// prefix, because a prefix every id carries says nothing.
	//
	// Revisions:
	//   - 2026-09-19 23:36: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
	//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
	//   - 2026-11-08 02:11: the aliases of SPINE and THREAD dropped (C107, Q247)
	FIRST_SPAWN = 1

	// REPORTER and LOGGER name what failed when a host's own reporter or
	// logger raises, so the error says whose bug it is rather than reading as
	// the script's.
	//
	// Revisions:
	//   - 2026-09-20 19:57: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:18: moved to package run from scheduler (C91w4, Q228)
	REPORTER = "reporter"
	LOGGER   = "logger"
)
