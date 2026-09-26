package scheduler

import (
	"context"

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
// Ended takes an error rather than a status, so whoever maps "this failed" to a
// value in some schema does it in one place, next to the schema. This package
// names no status anywhere.
//
// Printed is the third moment: a line a script printed, on the lane that
// printed it. It is here rather than carried separately because it is the
// same kind of thing - something a run tells its host - and a host collecting
// a run's output wants it beside the statuses.
type Reporter interface {
	Started(thread string, name string, attempt int32)
	Ended(thread string, name string, err error)
	Printed(thread string, msg string)
}

// _Key is the context key a reporter is carried under. Its own type, so
// nothing else can collide with it.
type _Key struct{}

// _Console is a reporter that only prints, for a host that wants a script's
// output and nothing else.
type _Console struct {
	print func(string)
}

// Started is empty: a console reports nothing but what was printed.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func (c *_Console) Started(thread string, name string, attempt int32) {
	// Empty
}

// Ended is empty: a console reports nothing but what was printed.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func (c *_Console) Ended(thread string, name string, err error) {
	// Empty
}

// Printed hands the line to the host.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func (c *_Console) Printed(thread string, msg string) {
	c.print(msg)
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

// WithPrinter returns a context carrying a reporter that only prints, for a
// host that wants a script's output and nothing else.
//
// One reporter per run: this and WithReporter set the same thing, and the
// later call wins. A host that wants both implements Printed on its reporter.
// Without either the interpreter's default stands, which writes to standard
// error.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation, carrying a function of its own
//   - 2026-09-21 09:46: a reporter, so a run tells its host one thing
func WithPrinter(ctx context.Context, print func(string)) context.Context {
	return WithReporter(ctx, &_Console{print: print})
}

// Reporting returns what this run reports to, or nil if nothing is listening.
//
// Nil rather than an error, and nil rather than a do-nothing reporter: a script
// run from a command line has no watcher, and that is the ordinary case rather
// than a degraded one. Every call site checks, and pays a nil check.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-21 08:09: read from the run, which every evaluation carries
func Reporting(thread *starlark.Thread) Reporter {
	locals, err := _Of(thread)
	if err != nil {
		return nil
	}

	return locals.run.into
}

// Number is the id of the lane the evaluation on thread runs on.
//
// The spine when a thread carries none, which is a thread nothing set up. An id
// is only ever used to group what is reported, so a wrong lane is a tidier
// failure than no answer at all.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-21 00:59: a thread id is a string that names its parent
//   - 2026-09-21 08:09: read from the one local
func Number(thread *starlark.Thread) string {
	locals, err := _Of(thread)
	if err != nil {
		return SPINE
	}

	return locals.thread
}

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

// LAMBDA is what the interpreter calls an anonymous function. Nothing refuses
// one; this is here so a reporter can tell that a name is not one.
const LAMBDA = "lambda"
