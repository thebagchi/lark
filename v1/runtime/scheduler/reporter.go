package scheduler

import (
	"go.starlark.net/starlark"
)

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
type Reporter interface {
	Started(thread string, line *Line)
	Ended(thread string, name string, err error)
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
type Line struct {
	Name    string
	Builtin string
	Attempt int32
	Count   int32
	Child   string
	Binding string
	Waits   []string
}

// _Lane is the id of the lane the evaluation on thread runs on.
//
// The spine when a thread carries none, which is a thread nothing set up. An id
// is only ever used to group what is reported, so a wrong lane is a tidier
// failure than no answer at all.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-21 00:59: a thread id is a string that names its parent
//   - 2026-09-21 08:09: read from the one local
//   - 2026-10-03 08:26: unexported as _Lane, nothing outside this package asking it
func _Lane(thread *starlark.Thread) string {
	locals, err := _Of(thread)
	if err != nil {
		return SPINE
	}

	return locals.thread
}

// Open tells whatever is watching the run on thread that a line has started,
// on the lane thread runs on.
//
// Through the run's guard, so a reporter that raises ends the run as one that
// raises on a spawn does, rather than failing whatever builtin reported the
// line. Nothing to tell when nothing is listening, or when thread carries no
// run.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation, from flow's _Began, so every builtin
//     reports a line one way
func Open(thread *starlark.Thread, line *Line) {
	locals, err := _Of(thread)
	if err != nil {
		return
	}

	locals.run._Tell(func() {
		locals.run.into.Started(locals.thread, line)
	})
}

// Close tells whatever is watching the run on thread how the line that called
// name ended, through the same guard as Open.
//
// Revisions:
//   - 2026-10-02 00:38: initial creation, from flow's _Finished, so every
//     builtin reports a line one way
func Close(thread *starlark.Thread, name string, err error) {
	locals, failure := _Of(thread)
	if failure != nil {
		return
	}

	locals.run._Tell(func() {
		locals.run.into.Ended(locals.thread, name, err)
	})
}

// LAMBDA is what the interpreter calls an anonymous function. Nothing refuses
// one; this is here so a reporter can tell that a name is not one.
const LAMBDA = "lambda"
