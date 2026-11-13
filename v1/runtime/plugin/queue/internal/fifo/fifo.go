// Package fifo holds the queue plugin's queue: the items it holds in order,
// the pops that wait for one, whether it is closed, the value a script sees it
// as, and the builtins that make one and work on one. The plugin type and its
// registration stay in package queue.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q417)
package fifo

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrClosed is returned for pushing to a queue that has been closed.
	//
	// A close says no more will come, and a push after it is a producer that
	// did not hear. The mistake is in the script, so the push stops the whole
	// run, as a second event.post does.
	//
	// Revisions:
	//   - 2026-11-13 01:26: initial creation (C131, Q412, Q414)
	ErrClosed = errors.New("this queue is closed")
)

// Queue is the items one thread hands to others, in the order they were pushed.
//
// It holds the items it was given and no copy of them: push freezes a value in
// place, the queue keeps that same value, and pop hands out that same value
// (Q420). The items sit in a ring that doubles up to the capacity, so a push and
// a pop cost the same however full the queue is, and a large capacity costs
// nothing until it is used. Every slot of the ring that does not hold an item is
// nil, so a popped item can be collected.
//
// The state is behind a mutex, so a queue stays usable when frozen, as a module
// global is.
//
// The pops that wait stand in a list, the one that has waited longest first
// (Q428). Only that one may take the next item, and an item is never taken out
// of the ring for a pop that may yet be cancelled. A push wakes the first
// waiter only. When that waiter has taken its item it wakes the next, if an
// item is left; a close wakes every waiter, and each then gives up its place
// or takes its turn. A waiter that is cancelled leaves the list and wakes the
// next in its place, and has taken nothing, so no item is lost or reordered. A
// pop that arrives while others wait stands behind them, whatever the ring
// holds. Each waiter is woken through a channel of one token, which a wake can
// fill without blocking, so a wake cannot be lost and cannot stop the waker.
//
// A zero Queue works and holds spec.CAPACITY items.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q408, Q417, Q420)
//   - 2026-11-13 09:38: a ring that doubles up to the capacity in place of a slice that is
//     compacted, and a list of waiting pops in place of one channel for all (C131, Q428)
type Queue struct {
	guard    sync.Mutex
	ring     []starlark.Value
	head     int
	count    int
	capacity int
	closed   bool
	waiting  list.List
	wakes    int
}

// _Waiter is one pop that waits. ready holds a token when the pop should look
// at the queue again. place is where it stands in the list.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q428)
type _Waiter struct {
	ready chan struct{}
	place *list.Element
}

// New is an empty queue that holds capacity items, or spec.CAPACITY when
// capacity is 0 (Q418).
//
// Returns unpack.ErrRange for a negative capacity. The ring is not allocated
// here: it grows as items arrive, so a large capacity costs nothing until it is
// used.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q411, Q418)
func New(capacity int) (*Queue, error) {
	if capacity < 0 {
		return nil, fmt.Errorf(
			"queue capacity is negative: %w",
			unpack.ErrRange,
		)
	}

	return &Queue{
		capacity: capacity,
	}, nil
}

// Push adds value at the back and never waits. It answers None, or the text
// spec.FULL when the queue already holds as many items as its capacity.
//
// The caller has made the value fit to be shared: Push keeps it as it is. If
// pops wait, the first of them is woken to take it.
//
// Returns ErrClosed when the queue is closed, and scheduler.ErrRun for a nil
// queue.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q409, Q412, Q420)
//   - 2026-11-13 09:38: wakes the first waiting pop only (C131, Q428)
func (q *Queue) Push(value starlark.Value) (starlark.Value, error) {
	if q == nil {
		return nil, _Nil()
	}

	q.guard.Lock()
	defer q.guard.Unlock()

	if q.closed {
		return nil, ErrClosed
	}

	if q.count >= q._Capacity() {
		return starlark.String(spec.FULL), nil
	}

	q._Put(value)
	q._Wake()

	return starlark.None, nil
}

// Pop takes the item at the front, and waits until there is one, or the queue
// is closed, or ctx is done.
//
// Answers the item and false; or nil and true when the queue is closed and
// holds nothing. A closed queue gives up its items first, in order, and each
// goes to one caller. Pops that wait are served in the order they began to wait
// (Q428).
//
// Returns scheduler.ErrCancelled wrapping the context's error when ctx is done
// first, because then there is no script left to hand an answer to. ctx is
// looked at before the queue is, and a pop takes its item only when it is its
// turn, so a cancelled caller takes no item and leaves the list at once.
// Returns scheduler.ErrRun for a nil queue or a nil ctx.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q416, Q419)
//   - 2026-11-13 09:38: waits in a list, the longest waiter first (C131, Q428)
func (q *Queue) Pop(ctx context.Context) (starlark.Value, bool, error) {
	if q == nil {
		return nil, false, _Nil()
	}

	if ctx == nil {
		return nil, false, fmt.Errorf(
			"nil context: %w",
			scheduler.ErrRun,
		)
	}

	var turn *_Waiter

	for {
		err := ctx.Err()
		if err != nil {
			q._Leave(turn)

			return nil, false, _Cancelled(err)
		}

		var (
			value  starlark.Value
			closed bool
		)

		value, closed, turn = q._Try(turn)
		if turn == nil {
			return value, closed, nil
		}

		select {
		case <-turn.ready:

		case <-ctx.Done():
		}
	}
}

// Close ends the queue: every waiting pop wakes, a pop of what is left still
// gets it, and a push is refused from now on. A second close does nothing.
//
// Returns scheduler.ErrRun for a nil queue.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q412)
//   - 2026-11-13 09:38: wakes the waiting pops through the list (C131, Q428)
func (q *Queue) Close() error {
	if q == nil {
		return _Nil()
	}

	q.guard.Lock()
	defer q.guard.Unlock()

	if q.closed {
		return nil
	}

	q.closed = true
	q._Wake()

	return nil
}

// Len is how many items the queue holds. A nil queue holds none.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q417)
func (q *Queue) Len() int {
	count, _, _ := q.State()

	return count
}

// Cap is how many items the queue holds at most. A nil queue holds none.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q417)
func (q *Queue) Cap() int {
	_, capacity, _ := q.State()

	return capacity
}

// State is how many items the queue holds, how many it can hold, and whether it
// is closed, all read under one lock. A nil queue holds none, can hold none and
// is open.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q427)
func (q *Queue) State() (int, int, bool) {
	if q == nil {
		return 0, 0, false
	}

	q.guard.Lock()
	defer q.guard.Unlock()

	return q.count, q._Capacity(), q.closed
}

// Waiting is how many pops wait on the queue. A nil queue has none.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q428)
func (q *Queue) Waiting() int {
	if q == nil {
		return 0
	}

	q.guard.Lock()
	defer q.guard.Unlock()

	return q.waiting.Len()
}

// Wakes is how many times the queue has woken a waiting pop, counted at each
// wake call, whether or not the pop already held a token. A test reads it to see
// that a push wakes the first waiter only. A nil queue has woken none.
//
// Revisions:
//   - 2026-11-13 11:25: initial creation (C131, Q428, Q314)
func (q *Queue) Wakes() int {
	if q == nil {
		return 0
	}

	q.guard.Lock()
	defer q.guard.Unlock()

	return q.wakes
}

// Slots is the ring as it stands: its length, and the item in each slot, nil
// for a slot that holds none. A test reads it to see that a popped item is let
// go. A nil queue has no slots.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q420)
func (q *Queue) Slots() []starlark.Value {
	if q == nil {
		return nil
	}

	q.guard.Lock()
	defer q.guard.Unlock()

	return append(
		[]starlark.Value(nil),
		q.ring...,
	)
}

// _Try is one look at the queue by a pop, under the guard. It answers the item,
// or closed when the queue is closed and empty, or the place in the list the
// pop must wait in.
//
// A pop takes an item only when no pop stands before it. One that has to wait
// is put at the back of the list the first time, and keeps its place.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q416, Q428)
func (q *Queue) _Try(turn *_Waiter) (starlark.Value, bool, *_Waiter) {
	q.guard.Lock()
	defer q.guard.Unlock()

	first := q.waiting.Len() == 0 || (turn != nil && q.waiting.Front() == turn.place)

	if q.count > 0 && first {
		value := q._Take()

		q._Depart(turn)
		q._Wake()

		return value, false, nil
	}

	if q.count == 0 && q.closed {
		q._Depart(turn)
		q._Wake()

		return nil, true, nil
	}

	if turn == nil {
		turn = &_Waiter{
			ready: make(
				chan struct{},
				1,
			),
		}
		turn.place = q.waiting.PushBack(turn)
	}

	return nil, false, turn
}

// _Leave takes a pop out of the list, because it was cancelled, and wakes the
// pop that is first now. It takes no item.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q428)
func (q *Queue) _Leave(turn *_Waiter) {
	if turn == nil {
		return
	}

	q.guard.Lock()
	defer q.guard.Unlock()

	q._Depart(turn)
	q._Wake()
}

// _Depart takes a pop out of the list. Called under the guard.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q428)
func (q *Queue) _Depart(turn *_Waiter) {
	if turn == nil || turn.place == nil {
		return
	}

	q.waiting.Remove(turn.place)

	turn.place = nil
}

// _Wake wakes the pops that may look at the queue: the first, when an item is
// there; all of them, when the queue is closed and empty, because each then
// answers closed. Called under the guard.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q428)
func (q *Queue) _Wake() {
	if q.waiting.Len() == 0 {
		return
	}

	if q.count > 0 {
		q._Signal(q.waiting.Front())

		return
	}

	if !q.closed {
		return
	}

	for place := q.waiting.Front(); place != nil; place = place.Next() {
		q._Signal(place)
	}
}

// _Signal puts a token in the channel of the waiter at place, unless one is
// there, and counts the wake. It never blocks. Called under the guard.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131, Q428)
func (q *Queue) _Signal(place *list.Element) {
	q.wakes++

	turn, ok := place.Value.(*_Waiter)
	if !ok {
		return
	}

	select {
	case turn.ready <- struct{}{}:

	default:
	}
}

// _Take removes the item at the front of the ring and answers it. The slot is
// set to nil, so the queue keeps no reference to the item. Called under the
// guard, with an item there.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q416, Q420)
//   - 2026-11-13 09:38: takes from a ring (C131, Q420)
func (q *Queue) _Take() starlark.Value {
	value := q.ring[q.head]
	q.ring[q.head] = nil
	q.head++
	q.count--

	if q.head == len(q.ring) {
		q.head = 0
	}

	return value
}

// _Put adds value at the back of the ring, which grows first when it is full.
// Called under the guard, with room checked.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q409, Q420)
//   - 2026-11-13 09:38: a ring that grows by doubling, in place of a slice that is compacted
//     (C131)
func (q *Queue) _Put(value starlark.Value) {
	if q.count == len(q.ring) {
		q._Grow()
	}

	at := q.head + q.count
	if at >= len(q.ring) {
		at -= len(q.ring)
	}

	q.ring[at] = value
	q.count++
}

// _Grow doubles the ring, up to the capacity, and sets the items in order from
// the first slot. The old ring is let go whole. Called under the guard, with the
// ring full and the capacity not reached.
//
// Revisions:
//   - 2026-11-13 09:38: initial creation (C131)
func (q *Queue) _Grow() {
	size := min(
		max(
			spec.DOUBLE*len(q.ring),
			spec.FIRST,
		),
		q._Capacity(),
	)

	var (
		grown = make(
			[]starlark.Value,
			size,
		)
		moved = copy(
			grown,
			q.ring[q.head:],
		)
	)

	copy(
		grown[moved:],
		q.ring[:q.head],
	)

	q.ring = grown
	q.head = 0
}

// _Capacity is how many items may be held, spec.CAPACITY for a zero queue.
// Called under the guard.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q418)
func (q *Queue) _Capacity() int {
	if q.capacity == 0 {
		return spec.CAPACITY
	}

	return q.capacity
}

// _Nil is the refusal for a queue that is not there.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q65, Q206)
func _Nil() error {
	return fmt.Errorf(
		"nil queue: %w",
		scheduler.ErrRun,
	)
}

// _Cancelled is the refusal for a caller whose context is done.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q416)
func _Cancelled(cause error) error {
	return fmt.Errorf(
		"%w: %w",
		scheduler.ErrCancelled,
		cause,
	)
}
