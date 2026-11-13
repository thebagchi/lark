package fifo

import (
	"fmt"
	"maps"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/spec"
)

var (
	// QUEUE_METHODS is what the attributes of a queue call, each bound to the
	// queue it is read from. Each is made with plugin.NewBuiltin, so a nil
	// argument is refused.
	//
	// Revisions:
	//   - 2026-11-13 01:27: initial creation (C131, Q410, Q417)
	QUEUE_METHODS = map[string]*starlark.Builtin{
		spec.PUSH: plugin.NewBuiltin(
			spec.NAME+"."+spec.PUSH,
			_Push,
		),
		spec.POP: plugin.NewBuiltin(
			spec.NAME+"."+spec.POP,
			_Pop,
		),
		spec.CLOSE: plugin.NewBuiltin(
			spec.NAME+"."+spec.CLOSE,
			_Close,
		),
	}
)

// String is what print shows: the items held, the capacity, and "closed" once
// the queue is closed, all read under one lock. The items themselves are not
// shown, as there may be many.
//
// Text decided by the user: <queue 3/64> and <queue 3/64 closed> (Q427).
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
//   - 2026-11-13 09:38: shows the items held, the capacity and closed (C131, Q427)
func (q *Queue) String() string {
	count, capacity, closed := q.State()

	if closed {
		return fmt.Sprintf(
			"<%s %d/%d closed>",
			spec.TYPE,
			count,
			capacity,
		)
	}

	return fmt.Sprintf(
		"<%s %d/%d>",
		spec.TYPE,
		count,
		capacity,
	)
}

// Type names this value's type, as a script's type() sees it.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
func (q *Queue) Type() string {
	return spec.TYPE
}

// Freeze is empty: the state of a queue is behind its mutex, so a frozen queue,
// a module global or one a spawn captured, still takes and gives items. What
// freezing protects a value from, a change from two threads at once, the mutex
// already does.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
func (q *Queue) Freeze() {
	// Empty
}

// Truth reports a queue as true, so a script can test one for presence.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
func (q *Queue) Truth() starlark.Bool {
	return starlark.True
}

// Hash refuses, because a queue is identity rather than value, as a thread
// handle is.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q408)
func (q *Queue) Hash() (uint32, error) {
	return 0, fmt.Errorf(
		"%s is unhashable",
		spec.TYPE,
	)
}

// Attr is the method name calls, bound to this queue, or nil when there is no
// such method.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q410)
func (q *Queue) Attr(name string) (starlark.Value, error) {
	method, ok := QUEUE_METHODS[name]
	if !ok {
		return nil, nil
	}

	return method.BindReceiver(q), nil
}

// AttrNames is the methods, as dir() lists them.
//
// Revisions:
//   - 2026-11-13 01:27: initial creation (C131, Q410)
func (q *Queue) AttrNames() []string {
	return slices.Sorted(maps.Keys(QUEUE_METHODS))
}
