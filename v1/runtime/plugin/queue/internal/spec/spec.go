// Package spec is internal, not public: the fixed words, names and numbers of the queue plugin.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q417)
package spec

// NAME is the module, and the names it holds. NEW makes a queue. PUSH, POP and
// CLOSE are the methods of a queue. MEMBER_FULL and MEMBER_CLOSED are the
// names of the two constants the module holds, whose texts are FULL and CLOSED.
// TYPE is what type() calls a queue.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q410, Q415, Q417)
const (
	NAME          = "queue"
	NEW           = "new"
	PUSH          = "push"
	POP           = "pop"
	CLOSE         = "close"
	MEMBER_FULL   = "FULL"
	MEMBER_CLOSED = "CLOSED"
	TYPE          = "queue"
)

// FULL is what a push answers when the queue holds as many items as its
// capacity. CLOSED is what a pop answers once the queue is closed and empty.
// Plain words, because they are what a script prints or compares with.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q415, Q419)
const (
	FULL   = "full"
	CLOSED = "closed"
)

// CAPACITY is how many items a queue holds when its maker names no capacity,
// or names 0. FIRST is how many slots the ring of a queue has when it first
// holds an item, or fewer when the capacity is fewer. DOUBLE is how many times
// larger the ring grows.
//
// Revisions:
//   - 2026-11-13 01:26: initial creation (C131, Q411, Q418)
//   - 2026-11-13 09:38: FIRST (C131)
const (
	CAPACITY = 64
	FIRST    = 8
	DOUBLE   = 2
)
