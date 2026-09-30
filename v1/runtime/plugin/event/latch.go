package event

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// EVENTS is the name a run keeps its events under.
	EVENTS = "event.latches"

	// LATCH is what one event is charged before anything it carries: a channel,
	// a map entry and the struct holding them. An estimate, like every figure
	// the budget uses, rounded up so the charge is never under.
	//
	// Charged where the event is made rather than where it is posted, because
	// either side makes one - a waiter naming an event nobody has posted is how
	// a waiter arrives first, and a script could name a million of them without
	// ever posting.
	LATCH = 256
)

// _Events is every event of one run.
//
// Per run rather than per thread, because the whole point is that another
// thread sees it. Held through scheduler.Shared, which is where per-run state
// already lives, so two runs in one process never share one.
type _Events struct {
	guard sync.Mutex
	held  map[string]*_Latch
}

// _Latch is one thing that will happen, or has.
//
// done is closed when it is posted, and that is the whole mechanism. A closed
// channel is readable by any number of receivers, so one close releases every
// waiter without a list of them; and it stays readable, so a waiter arriving
// afterwards is not waiting at all. sync.Cond does the first and not the second,
// and cannot appear in a select - which a wait watching a timeout and a
// cancellation at once has to do.
type _Latch struct {
	done   chan struct{}
	guard  sync.Mutex
	value  starlark.Value
	posted bool
}

// _Of is this run's events, made on first use.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
func _Of(thread *starlark.Thread) (*_Events, error) {
	return scheduler.Shared(thread, EVENTS, func() *_Events {
		return &_Events{held: map[string]*_Latch{}}
	})
}

// _Named is the event of that name, made if this is the first mention of it.
//
// An event exists as soon as it is named, by either side, which is what lets a
// waiter arrive before the poster does - and is why making one is charged here
// rather than where it is posted.
//
// Returns ErrMemory when the run cannot afford another, having made nothing.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
func (e *_Events) _Named(budget *scheduler.Budget, name string) (*_Latch, error) {
	e.guard.Lock()
	defer e.guard.Unlock()

	held, found := e.held[name]
	if found {
		return held, nil
	}

	err := budget.Charge(LATCH + int64(len(name)))
	if err != nil {
		return nil, err
	}

	held = &_Latch{done: make(chan struct{})}
	e.held[name] = held

	return held, nil
}

// _Post posts the event named, or says why it cannot be.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
func (e *_Events) _Post(budget *scheduler.Budget, name string, value starlark.Value) error {
	held, err := e._Named(budget, name)
	if err != nil {
		return err
	}

	return held._Post(budget, value)
}

// _Post hands this latch its value and releases every waiter.
//
// Charged for what it carries and never credited, because an event is posted
// once and nothing deletes it - so what it holds is held for the life of the
// run, which is the reason the store charges too.
//
// Returns ErrPosted when it has already been posted.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
func (l *_Latch) _Post(budget *scheduler.Budget, value starlark.Value) error {
	l.guard.Lock()
	defer l.guard.Unlock()

	if l.posted {
		return fmt.Errorf("%w", ErrPosted)
	}

	err := budget.Charge(deep.Size(value))
	if err != nil {
		return err
	}

	l.value = value
	l.posted = true

	close(l.done)

	return nil
}

// _Await waits for this latch, for the time given, or for the caller to be
// cancelled - whichever comes first.
//
// Answers with the value and whether the wait ran out. Returns ErrCancelled
// wrapping the context's error when the caller went first, because then there is
// nothing left to hand an answer to.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
func (l *_Latch) _Await(ctx context.Context, bound time.Duration) (starlark.Value, bool, error) {
	waited := time.NewTimer(bound)
	defer waited.Stop()

	select {
	case <-l.done:
		l.guard.Lock()
		defer l.guard.Unlock()

		return l.value, false, nil

	case <-waited.C:
		return starlark.None, true, nil

	case <-ctx.Done():
		return nil, false, fmt.Errorf("%w: %w", scheduler.ErrCancelled, ctx.Err())
	}
}
