// Package queue gives a script the queue module. Importing it is what enables
// it.
//
// One thread hands many values to others, in order.
//
//	q = queue.new()
//
//	q.push({"id": 1})
//
//	value, err = q.pop()
//	if err:
//	    fail(err)
//
// queue.new() makes a queue that holds 64 items, and queue.new(n) one that holds
// n. The capacity is 1 or more, or 0 for the default; a negative number or a
// value that is not an int stops the run. The queue is an object a script
// passes to the threads it spawns, keeps in a global, and returns from a function.
// It works when frozen, because its state sits behind a mutex.
//
// q.push(value) never waits. It answers None, or queue.FULL when the queue holds
// as many items as its capacity, so the producer decides what a full queue
// means. It freezes the value and keeps that same value: nothing is copied.
// A value that is not data stops the run, as event.post does. A push after
// q.close() stops the run with ErrClosed.
//
// q.pop() takes the item at the front and waits until there is one. It answers
// the pair (value, None), or (None, queue.CLOSED) once the queue is closed and
// empty. It takes no parameter: a run cancel, or a timeout() around the call,
// ends the wait. Each item goes to one popper, the one that has waited longest,
// and a closed queue gives up the items it still holds first, in order. A pop
// inside a state.update is refused with scheduler.ErrNested, as event.wait is.
//
// q.close() wakes every waiting pop. A second close does nothing.
//
// print(q) shows the items held and the capacity, and closed once it is closed:
// <queue 3/64>, <queue 0/64 closed>.
//
// A queue is not data, so event.post, state.set and q.push refuse one as they
// refuse a thread handle.
//
// A module rather than names a script always has, so a host that does not want
// a script able to block on another thread leaves the import out. Nothing
// bounds how long a pop lasts except the script's own close, timeout() or cancel.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408-Q429)
package queue

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/fifo"
	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/spec"
)

var (
	// ErrClosed is fifo.ErrClosed.
	//
	// Revisions:
	//   - 2026-11-13 01:27: initial creation (C131, Q414)
	ErrClosed = fifo.ErrClosed
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
func init() {
	plugin.Register(&_Queue{})
}

// _Queue is the plugin. Empty: each queue holds its own items.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
type _Queue struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
func (p *_Queue) Name() string {
	return spec.NAME
}

// Values returns the queue module.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
func (p *_Queue) Values() starlark.StringDict {
	return fifo.Values()
}
