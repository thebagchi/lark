package common

// Evaluation is the evaluation running on a thread, as a builtin that needs
// more than one thing from it sees it.
//
// It exposes one thing, Lock: the run's lock for a name, taken on behalf of
// this evaluation. The scheduler's internal package holds the type that
// implements it and the rest of what an evaluation carries (Q119, Q127).
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:55: moved from scheduler, as the interface of the Evaluation struct (C91w5,
//     Q119, Q127)
type Evaluation interface {
	// Lock takes the run's lock for name and returns the Hold that releases
	// it. Release it with scheduler.Release.
	Lock(name string) (Hold, error)
}

// ForkOption sets one thing on the evaluation that scheduler.Fork starts,
// given the evaluation that is starting it (parent) and the one it starts
// (child).
//
// Limit (Q128): a ForkOption receives an Evaluation that exposes only Lock.
// A function a host writes can therefore do nothing to either evaluation but
// lock. The four constructors of package scheduler, Attempt, Catching, Inline
// and Labelled, are the only intended source of options: each asserts the
// concrete type inside the scheduler and sets what it needs.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:55: renamed from scheduler.Option and moved to common (C91w5, Q119, Q128)
//   - 2026-11-12 18:58: renamed from BesideOption (C129, Q396)
type ForkOption func(
	parent Evaluation,
	child Evaluation,
)

// Hold is one hold of a run's lock, the value Lock returns, and what
// scheduler.Release frees.
//
// It is data: an opaque Owner and a Token, with exported fields because the
// scheduler builds one and common may not import it. Nothing outside the
// scheduler may read or set them. Owner names what holds the lock, Token names
// which hold of it this is, so a Hold kept past its use cannot free a later
// hold. A zero Hold, the one Lock returns with an error, holds nothing, and
// copies of a Hold stand for the same hold.
//
// Hold is a value, not a pointer: it never leaves the function boundary, so
// Lock and Release make no heap allocation (`go build -gcflags=-m` says it does
// not escape). Owner holds a pointer, which an interface carries without
// allocating.
//
// Revisions:
//   - 2026-11-05 22:40: initial creation, in place of the func() that Lock returned (C58
//     fix 2, Q201)
//   - 2026-11-05 23:08: marked as a value type for the pointer check (C58 fix 2, Q203)
//   - 2026-11-05 23:29: states why it is a value, not a pointer (C58 fix 3, Q204)
//   - 2026-11-07 19:55: moved from scheduler to common as data, with no method; scheduler.Release
//     frees it
//     (C91w5, Q229)
//
//convention:value
type Hold struct {
	Owner any
	Token uint64
}
