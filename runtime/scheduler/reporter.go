package scheduler

import (
	"context"

	"go.starlark.net/starlark"
)

// REPORTER_KEY names what a run leaves on a thread so a function starting or
// stopping can find what to tell.
const REPORTER_KEY = "scheduler.reporter"

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
// Ended takes an error rather than a status, so whoever maps "this failed" to a
// value in some schema does it in one place, next to the schema. This package
// names no status anywhere.
type Reporter interface {
	Started(thread int32, name string, attempt int32)
	Ended(thread int32, name string, err error)
}

// WithReporter returns a context carrying the reporter a run should tell what
// it is doing.
//
// The context, because that is what a caller already hands to a run and the
// only thing that reaches Begin from outside this package.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func WithReporter(ctx context.Context, into Reporter) context.Context {
	return context.WithValue(ctx, _Key{}, into)
}

// Reporting returns what this run reports to, or nil if nothing is listening.
//
// Nil rather than an error, and nil rather than a do-nothing reporter: a script
// run from a command line has no watcher, and that is the ordinary case rather
// than a degraded one. Every call site checks, and pays a nil check.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func Reporting(thread *starlark.Thread) Reporter {
	into, ok := thread.Local(REPORTER_KEY).(Reporter)
	if !ok {
		return nil
	}

	return into
}

// Number is the thread number this evaluation runs under.
//
// The spine when a thread carries none, which is a thread nothing set up. A
// number is only ever used to group what is reported, so a wrong lane is a
// tidier failure than no answer at all.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func Number(thread *starlark.Thread) int32 {
	number, ok := thread.Local(THREAD_KEY).(int32)
	if !ok {
		return SPINE
	}

	return number
}

// _Key is the context key a reporter is carried under. Its own type, so
// nothing else can collide with it.
type _Key struct{}

// _Reporter returns the reporter carried by ctx, or nil.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func _Reporter(ctx context.Context) Reporter {
	into, ok := ctx.Value(_Key{}).(Reporter)
	if !ok {
		return nil
	}

	return into
}
