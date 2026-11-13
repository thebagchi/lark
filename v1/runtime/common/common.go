// Package common holds the data that more than one package of the runtime
// shares: constants and the types that carry state. It holds data only: no
// function, no method, no logic.
//
// POLL and DEPTH are the constants. Source and Loader are the script a host
// hands over and the way its modules are fetched. Trail is the type: the state of the
// shared walker. The behavior that reads and writes a Trail lives in the
// package trail and below it, so this package stays a place any package can
// import without taking a dependency on behavior.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation, with POLL
//   - 2026-11-02 12:28: DEPTH, INLINE and Trail, for the shared walker (C26)
//   - 2026-11-02 22:32: the memory watch's constants and its two types (C27)
//   - 2026-11-07 18:16: Plugin and Checking, interfaces with no body (C91w1)
//   - 2026-11-07 18:30: Source and Loader, moved from script (C91w2)
//   - 2026-11-07 19:14: Settings, Line, Reporter and the scheduler's shared constants,
//     moved from scheduler (C91w4)
//   - 2026-11-07 19:18: Field, BIG and LITTLE, moved from integer (Q124, C91w8b)
//   - 2026-11-07 19:55: Handle, Evaluation, BesideOption and Hold, moved from scheduler (C91w5)
//   - 2026-11-07 23:04: Artifact, Given, RunOption, CompileOption and CompileSettings, moved
//     from artifact (C91w6)
//   - 2026-11-08 00:57: spelling names and OPTIONS, from spelling and dialect (Q244, C91w12d)
//   - 2026-11-08 00:59: QUOTE_LIMIT, ANONYMOUS and BITS_PREFIX, moved from unpack and binary
//     (C91w12c, Q244, Q246)
package common

import "time"

// POLL is how much work runs between two looks at the context.
//
// Two units share the number, and a reader must not take it for one of them.
// In the scheduler it counts interpreter steps: a thread looks at its context
// every POLL steps, and again after every builtin call returns. In the shared
// walker (package trail) it counts elements: the walk looks at the context
// every POLL elements.
//
// 1,024 steps are about 3 to 10 microseconds of work. A cancelled thread runs
// at most about POLL more steps before a poll sees the cancel. Check: the test
// TestCancelOn_StopsWithinPoll in package scheduler.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-02 12:28: names the walker as it now exists
const (
	POLL = 1024
)

// DEPTH is the deepest nesting a walk over a script value accepts.
//
// The number protobuf and encoding/json use: protowire.DefaultRecursionLimit
// and maxNestingDepth are both 10,000, so a value deeper than this could not
// cross the wire or be read back. Check: the test TestEnter_DepthLimit in
// package trail_test.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation
const (
	DEPTH = 10000
)

// INLINE is how many containers a Trail holds without allocating a slice.
//
// A document 16 levels deep costs the walk no allocation for its path.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation
const (
	INLINE = 16
)

// Trail is the state of one walk over a script value.
//
// It carries the containers on the current path, the depth, the count of
// elements since the last look at the context, and that look. Make one with
// trail.New or trail.Bounded; read and write it only through the package
// trail. A Trail belongs to one walk on one goroutine and is not safe to
// share.
//
// The fields:
//
//   - Path holds the containers entered and not yet left, oldest first. A nil
//     entry stands for a value that cannot hold itself, a tuple. It is empty
//     when Cycles is false.
//   - Inline is the storage Path starts in.
//   - Depth is the count of containers entered and not yet left.
//   - Count is the count of elements since the last look at the context.
//   - Cycles is true when entering a container already on Path is an error. A
//     walk that copes with a cycle itself leaves it false.
//   - Err is the context's Err method, or nil when nothing can cancel the walk.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation
type Trail struct {
	Path   []any
	Inline [INLINE]any
	Depth  int
	Count  int
	Cycles bool
	Err    func() error
}

// The memory watch's numbers, in the order a run meets them.
//
// MEMORY_PERCENT is the share of memory in use at which the watch trips.
// WATCH_INTERVAL is how often the watch reads memory: one small read, and a
// script that grows by
// a gigabyte a second grows about 100 MB between two reads. WATCH_GRACE is how
// long a tripped run has to end before the host's abandon function is called.
// Check: TestStart_GraceIsHalfASecond in package memwatch_test.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
const (
	MEMORY_PERCENT = 95
	WATCH_INTERVAL = 100 * time.Millisecond
	WATCH_GRACE    = 500 * time.Millisecond
)

// The numbers of the size guard, which refuses a script-given size before any
// allocation (C46).
//
// ROOM_SKIP is the size below which the guard reads no memory. Reading the
// machine and the cgroup costs about as much as making and filling a buffer of
// this size, so a smaller buffer is cheaper to make than to check. A loop of
// small buffers is the memory watch's work, not the guard's.
// ROOM_CEILING is the size above which the guard refuses when memory cannot be
// read: 1 GiB, as decided (Q56).
// Check: TestFit and TestFit_Unreadable in package watch_test.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
const (
	ROOM_SKIP    = 1 << 20
	ROOM_CEILING = 1 << 30
)

// MemoryReading is memory in use and the memory there is, both in bytes.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
type MemoryReading struct {
	Used  uint64
	Total uint64
}

// MemoryWatch says how a run watches memory. A nil *MemoryWatch is no watch.
//
// Make one with the zero value and set only what differs from the default; read
// it only through the package memwatch.
//
//   - Percent is the share of Total that may be in use. Zero means
//     MEMORY_PERCENT.
//   - Interval is the time between reads. Zero means WATCH_INTERVAL.
//   - Grace is how long a tripped run has to end. Zero means WATCH_GRACE.
//   - Source returns the reading. Nil means the machine and the cgroup of this
//     process; a test sets it to inject a reading.
//   - Abandon, when set, is called with the trip error if the run has not ended
//     Grace after the trip. Nil leaves the process alone.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
type MemoryWatch struct {
	Percent  float64
	Interval time.Duration
	Grace    time.Duration
	Source   func() (*MemoryReading, error)
	Abandon  func(error)
}

// Spawn is one spawn written in a script: the def it is written in, and the
// function it starts. Two spawns of one function in one def are two values
// that are equal.
//
// It is what a graph needs to add the functions a stop kept a run from
// starting. Caller and Target are names as the run reports them.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
type Spawn struct {
	Caller string
	Target string
}

// Loader fetches the source of a module a script asked to load, and says what
// that module is actually called.
//
// A host serving scripts from a directory, an archive, a database or a test
// map implements this without having to be, or to own, a file system.
//
// Resolving is separate from fetching, and the split keeps two properties: a
// module reached by two routes is fetched once, and a cycle is refused without
// fetching anything. Both need to know what a spelling means before deciding
// whether to ask for it.
//
// Resolve is given the module doing the loading and the spelling it used, and
// returns the identity the module is filed under. Two spellings that reach one
// module must resolve to one name, or it is read twice; one spelling that
// reaches two modules must resolve to two, or the wrong one is reused. It is
// expected to be cheap - a path join, a key normalisation - because it runs for
// every load whether or not a fetch follows.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:30: moved here from script (C91w2)
type Loader interface {
	Resolve(
		from string,
		target string,
	) (string, error)
	Load(name string) ([]byte, error)
}

// Source is a script: the main script's name and text, and the Loader that
// finds the modules it loads.
//
// A struct rather than three parameters, so the main script and the modules it
// reaches arrive as one thing. Exported fields, because a caller in another
// package builds one and nothing here keeps them in step. A nil Loader reads a
// module as a file beside the one that loaded it.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:30: moved here from script (C91w2)
type Source struct {
	Entry  string
	Text   []byte
	Loader Loader
}
