package common

// Reporter is told when a function starts and when it stops, by the scheduler
// that runs it.
//
// Declared here rather than by whoever implements it, and that is the whole
// reason this file exists. Something watching a run needs to know what its
// functions are doing; it cannot reach in, because a run is unreachable once
// the call that made it returns. It cannot be imported either - a watcher
// imports the artifact package, which imports this one, so this one importing
// back would be a cycle.
//
// So this package says what it will tell, in its own terms, and a watcher
// implements it. Two methods, because there are two moments - not because a
// third might be useful later.
//
// Started takes a Line, the facts of the line that started. Ended takes an
// error rather than a status, so whoever maps "this failed" to a value in some
// schema does it in one place, next to the schema. This package names no status
// and no kind anywhere: a line carries the name of the builtin that reported
// it, and the schema's word for that is the watcher's to choose.
//
// What a script prints is not a moment of the run, and is not here: it is the
// run's transcript, which goes where WithTranscript says.
//
// Revisions:
//   - 2026-09-20 19:57: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:14: moved to package common (C91w4)
type Reporter interface {
	Started(
		thread string,
		line *Line,
	)
	Ended(
		thread string,
		name string,
		err error,
	)
}

// Line is one line a thread reports starting.
//
// Name is the function it called, empty for a join, a sleep or a cancel, and
// for a lambda that names no function the script defines. Builtin is the
// builtin that reported it - spawn, join, cancel, sleep, repeat, retry,
// timeout, or if and match for a branch - and empty for a plain call. Attempt
// is which attempt a repeat or a retry is on, and Count how many it makes or
// may make. Child is the thread a spawn started, Binding the name the script
// gave that spawn, and Waits the threads a join waits on.
//
// A struct rather than seven parameters, because a line has that many facts and
// most lines set two of them. Exported fields, because core and flow build one
// and a watcher reads it, as go.md allows a plain data struct handed across a
// package boundary.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:14: moved to package common (C91w4)
type Line struct {
	Name    string
	Builtin string
	Attempt int32
	Count   int32
	Child   string
	Binding string
	Waits   []string
}

// ATTR_THREAD and ATTR_FUNCTION are the attributes a printed line is logged
// with: the lane that printed it, and the function that did.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:14: moved to package common (C91w4)
const (
	ATTR_THREAD   = "thread"
	ATTR_FUNCTION = "function"
)

// NO_ATTEMPT is what an evaluation outside a repeat or a retry reports.
//
// Revisions:
//   - 2026-09-20 19:57: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:14: moved to package common (C91w4)
const (
	NO_ATTEMPT = 0
)
