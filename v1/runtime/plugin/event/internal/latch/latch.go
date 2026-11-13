// Package latch holds the posting side of the event plugin: the events of a run,
// the latch that one event waits on and posts to, and the builtins that post and
// bound a wait. The plugin type, its registration and the wait builtins stay in
// package event.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package latch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.starlark.net/starlark"

	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrPosted is returned for posting an event that has already been posted.
	//
	// A latch says a thing happened, and a thing happens once. Posting twice is
	// either two different things sharing a name or the same thing reported
	// twice, and both are mistakes a script author would rather hear about.
	//
	// Revisions:
	//   - 2026-09-30 21:12: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event (C120b, Q315)
	ErrPosted = errors.New("this event has already been posted")
)

// PostArgs reads the name and the value a post was given.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _PostArgs to PostArgs,
//     for the tests in tests/ (C120b, Q315, Q314)
func PostArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, starlark.Value, error) {
	var (
		name  string
		value starlark.Value
	)

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
		&name,
		&value,
	)
	if err != nil {
		return "", nil, err
	}

	return name, value, nil
}

// WaitBound reads the name and the duration a wait was given.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-06 00:10: names a nil builtin ANONYMOUS rather than panicking (C100b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _WaitBound to WaitBound,
//     for the tests in tests/ (C120b, Q315, Q314)
func WaitBound(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, time.Duration, error) {
	var (
		name  string
		given starlark.Value
	)

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
		&name,
		&given,
	)
	if err != nil {
		return "", 0, err
	}

	bound, err := scheduler.Duration(given)
	if err != nil {
		return "", 0, fmt.Errorf(
			"%s %s: %w",
			shared.Called(fn),
			unpack.QuotePrefix(name),
			err,
		)
	}

	return name, bound, nil
}

// Events is every event of one run.
//
// Per run rather than per thread, because the whole point is that another
// thread sees it. Held through scheduler.Shared, which is where per-run state
// already lives, so two runs in one process never share one.
//
// Revisions:
//   - 2026-09-30 21:12: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _Events to Events;
//     renamed the fields held to Held, for the tests in tests/ (C120b, Q315, Q314)
type Events struct {
	guard sync.Mutex
	Held  map[string]*Latch
}

// Latch is one thing that will happen, or has.
//
// done is closed when it is posted, and that is the whole mechanism. A closed
// channel is readable by any number of receivers, so one close releases every
// waiter without a list of them; and it stays readable, so a waiter arriving
// afterwards is not waiting at all. sync.Cond does the first and not the second,
// and cannot appear in a select - which a wait watching a timeout and a
// cancellation at once has to do.
//
// Revisions:
//   - 2026-09-30 21:12: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _Latch to Latch; renamed
//     the fields guard to Guard, done to Done, for the tests in tests/ (C120b, Q315, Q314)
type Latch struct {
	Guard  sync.Mutex
	Done   chan struct{}
	value  starlark.Value
	posted bool
}

// Named is the event of that name, made if this is the first mention of it.
//
// An event exists as soon as it is named, by either side, which is what lets a
// waiter arrive before the poster does.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
//   - 2026-11-01 13:00: charges nothing and cannot fail, the budget being gone
//   - 2026-11-06 00:10: a nil receiver answers nil, and a zero _Events makes its map
//     (C100b, B134, B135)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _Named to Named, for the
//     tests in tests/ (C120b, Q315, Q314)
func (e *Events) Named(name string) *Latch {
	if e == nil {
		return nil
	}

	e.guard.Lock()
	defer e.guard.Unlock()

	if e.Held == nil {
		e.Held = map[string]*Latch{}
	}

	held, found := e.Held[name]
	if found {
		return held
	}

	held = &Latch{
		Done: make(chan struct{}),
	}
	e.Held[name] = held

	return held
}

// Post posts the event named, or says why it cannot be.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
//   - 2026-11-01 13:00: takes no budget
//   - 2026-11-06 00:10: a nil receiver is ErrNoCallee, through the nil latch (C100b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _Post to Post, for the
//     tests in tests/ (C120b, Q315, Q314)
func (e *Events) Post(
	name string,
	value starlark.Value,
) error {
	return e.Named(name).Post(value)
}

// Post hands this latch its value and releases every waiter.
//
// Returns ErrPosted when it has already been posted.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
//   - 2026-11-01 13:00: charges nothing for the value
//   - 2026-11-06 00:10: a nil receiver is ErrNoCallee, and a zero latch makes the channel it
//     closes (C100b, B136)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _Post to Post, for the
//     tests in tests/ (C120b, Q315, Q314)
func (l *Latch) Post(
	value starlark.Value,
) error {
	if l == nil {
		return _NilLatch()
	}

	l.Guard.Lock()
	defer l.Guard.Unlock()

	if l.posted {
		return fmt.Errorf(
			"%w",
			ErrPosted,
		)
	}

	l.value = value
	l.posted = true

	if l.Done == nil {
		l.Done = make(chan struct{})
	}

	close(l.Done)

	return nil
}

// _NilLatch is the refusal for a latch that is not there.
//
// Revisions:
//   - 2026-11-06 00:10: initial creation (C100b, B135, B136)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func _NilLatch() error {
	return fmt.Errorf(
		"nil latch: %w",
		scheduler.ErrRun,
	)
}

// Await waits for this latch, for the time given, or for the caller to be
// cancelled - whichever comes first. A latch already posted is not waited for.
//
// Answers with the value and whether the wait ran out. Returns scheduler.ErrCancelled
// wrapping the context's error when the caller went first, because then there is
// nothing left to hand an answer to.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
//   - 2026-09-30 21:50: looks at the latch before arming the timer, so a wait
//     shorter than getting here cannot miss a post that came first
//   - 2026-11-06 00:10: reads done once, under the guard, so a write to it cannot race the
//     wait; a nil receiver is ErrNoCallee (C100b, B137, Q207)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _Await to Await, for the
//     tests in tests/ (C120b, Q315, Q314)
func (l *Latch) Await(
	ctx context.Context,
	bound time.Duration,
) (starlark.Value, bool, error) {
	if l == nil {
		return nil, false, _NilLatch()
	}

	done := l._Done()

	// Before the timer, because a select choosing between cases that are both
	// ready picks one at random. A bound already run out - zero, or anything
	// shorter than reaching the select - answered that the wait expired about
	// half the time, for an event posted before the wait began.
	select {
	case <-done:
		return l.Value(), false, nil

	default:
	}

	waited := time.NewTimer(bound)
	defer waited.Stop()

	select {
	case <-done:
		return l.Value(), false, nil

	case <-waited.C:
		return starlark.None, true, nil

	case <-ctx.Done():
		return nil, false, fmt.Errorf(
			"%w: %w",
			scheduler.ErrCancelled,
			ctx.Err(),
		)
	}
}

// Value is what this latch was posted with, read once done is closed. A nil
// latch has none.
//
// Revisions:
//   - 2026-09-30 21:50: initial creation
//   - 2026-11-06 00:10: a nil receiver answers None (C100b, B138)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event and renamed _Value to Value, for the
//     tests in tests/ (C120b, Q315, Q314)
func (l *Latch) Value() starlark.Value {
	if l == nil {
		return starlark.None
	}

	l.Guard.Lock()
	defer l.Guard.Unlock()

	return l.value
}

// _Done is the channel this latch closes when posted, made when a zero latch has
// none. Read under the guard, so it cannot race a write to it.
//
// Revisions:
//   - 2026-11-06 00:10: initial creation (C100b, B136, B137, Q207)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/event (C120b, Q315)
func (l *Latch) _Done() <-chan struct{} {
	l.Guard.Lock()
	defer l.Guard.Unlock()

	if l.Done == nil {
		l.Done = make(chan struct{})
	}

	return l.Done
}
