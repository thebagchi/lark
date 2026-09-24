package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/guard"
)

var (
	// ErrNoRun is returned when a builtin is called on a thread no run set up.
	ErrNoRun = errors.New("no run on this thread")

	// ErrNotLocal is returned when two plugins claim one key for values of
	// different types.
	ErrNotLocal = errors.New("a run local holds another type")
)

const (
	// LOCALS_KEY names the one thing a run leaves on an interpreter thread. A
	// Starlark builtin is handed only a thread, so this is how every builtin
	// here finds the evaluation it belongs to.
	//
	// One local rather than several, so a child inherits everything by copying
	// a struct and nothing is inherited by accident or dropped by omission.
	LOCALS_KEY = "scheduler.locals"

	// SPINE is the entry point's thread id, THREAD prefixes every id, and
	// FIRST_SPAWN is the ordinal the first child of any thread takes.
	//
	// An id names its parent: the spine's children are thread_1 and thread_2,
	// and thread_1's own first child is thread_1_1. The spine contributes no
	// prefix, because a prefix every id carries says nothing.
	SPINE       = "thread_0"
	THREAD      = "thread_"
	FIRST_SPAWN = 1

	// NO_ATTEMPT is what an evaluation outside a repeat or a retry reports.
	NO_ATTEMPT = 0

	// REPORTER names what failed when a host's own reporter raises, so the
	// error says whose bug it is rather than reading as the script's.
	REPORTER = "reporter"
)

// _Run is one entry point's execution.
//
// The context lives here rather than being passed, because a Starlark builtin
// is handed only a thread and a builtin is where spawn runs. go.md forbids a
// context on a struct that outlives the call it was scoped to; a _Run is
// created by a call into an artifact and discarded when that call returns, so
// it does not.
//
// There is no list of live handles. Every evaluation's context descends from
// ctx, so cancelling ctx is what cancels everything, and a list of what to
// cancel would be a second copy of that fact - one that a spawn made during
// teardown was measured not to be on.
type _Run struct {
	ctx   context.Context
	stop  context.CancelFunc
	group sync.WaitGroup
	into  Reporter

	mutex   sync.Mutex
	outcome error
	ordinal map[string]int32
	shared  map[string]any
	locks   map[string]chan struct{}
}

// _Locals is everything one evaluation carries: which run it belongs to,
// which lane it reports on, the context that cancels it, the attempt it is
// counted as, how deeply it is inside something catching assertions, and the
// name it is inside an update of.
//
// A child evaluation starts as a copy of its parent's, which is what makes
// "inside" mean the same thing whether the child is a spawn, an attempt or a
// bounded call. inside is one name, not a list: a second update is refused,
// so an evaluation is never inside two.
//
// holding says whether this evaluation is the one that took the name, rather
// than one that inherited the fact from whoever did. Both are refused a lock,
// and they are refused for different reasons - one is already updating, the
// other was merely started while somebody else was - so the refusal can say
// which. A child never holds: the lock belongs to the evaluation that took it.
type _Locals struct {
	run      *_Run
	thread   string
	ctx      context.Context
	attempt  int32
	catching int
	inside   string
	holding  bool
}

// Begin attaches a new run to thread and returns the function that ends it.
//
// This is the whole of what another package needs from this one: a call that
// wants spawn, join and cancel to work on its thread calls Begin, defers what
// it returns, and evaluates in between.
//
// Ending a run cancels its context, which every evaluation's context descends
// from, and then waits for every goroutine it started. Cancelling first is the
// order that matters: a child that spawns while the run is being torn down
// derives from a context that is already cancelled, so nothing it starts can
// escape. Measured the other way round, a child that kept spawning held the
// run open on 200 of 200 runs.
//
// Revisions:
//   - 2026-09-19 22:39: initial creation
//   - 2026-09-20 01:39: takes the entry point's name and reports the spine
//     starting and stopping, since this is where thread 0 is set up
//   - 2026-09-21 08:09: cancels the run's context before waiting, rather than
//     each handle it knew of; leaves one local on the thread; sets what print
//     reaches
//   - 2026-09-21 09:46: print reaches the reporter, which is the one thing a
//     run tells its host
func Begin(ctx context.Context, thread *starlark.Thread, name string) func() {
	inner, stop := context.WithCancel(ctx)

	run := &_Run{
		ctx:     inner,
		stop:    stop,
		into:    _Reporter(ctx),
		ordinal: make(map[string]int32),
		shared:  make(map[string]any),
		locks:   make(map[string]chan struct{}),
	}

	thread.SetLocal(LOCALS_KEY, &_Locals{run: run, thread: SPINE, ctx: inner})
	thread.Print = run._Print()

	watching := _CancelOn(inner, thread)

	run._Tell(func() {
		run.into.Started(SPINE, name, NO_ATTEMPT)
	})

	return func() {
		watching()
		stop()
		run.group.Wait()

		run._Tell(func() {
			run.into.Ended(SPINE, name, run._Outcome())
		})
	}
}

// _Print is what a thread of this run hands the interpreter for print, or nil
// when nothing is listening, which leaves the library's default.
//
// A printed line is reported on the lane that printed it, like a start or an
// end, and through the same guard: a reporter that raises on a print ends the
// run as one that raises on a start does.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-09-21 09:46: tells the reporter, on the printing thread's lane
func (r *_Run) _Print() func(*starlark.Thread, string) {
	if r.into == nil {
		return nil
	}

	return func(thread *starlark.Thread, msg string) {
		r._Tell(func() {
			r.into.Printed(Number(thread), msg)
		})
	}
}

// _Tell runs one report, and ends the run if the reporter raises.
//
// A reporter is a host's code running on a thread of this runtime's. Left
// unguarded here it leaves Invoke by a path Invoke does not recover, and for a
// caller that runs an artifact directly it takes the process down.
//
// It ends the run rather than being absorbed. A run whose report was never made
// has not been observed, and reporting success for it tells a host that
// everything worked including the part that did not.
//
// Revisions:
//   - 2026-09-20 12:00: initial creation
func (r *_Run) _Tell(report func()) {
	if r.into == nil {
		return
	}

	blown := guard.Contained(report)
	if blown != nil {
		r._End(fmt.Errorf("%s: %w", REPORTER, blown))
	}
}

// _End records the run's outcome and stops everything it started.
//
// A run has one outcome, and it is whatever was recorded first. Everything that
// happens afterwards - a spinning sibling cancelled, a thread told to stop
// between instructions, an evaluation unwinding - is the shutdown, not the
// reason for it.
//
// Revisions:
//   - 2026-09-20 00:07: initial creation, as _Fail
//   - 2026-09-20 00:12: named for what it records rather than what it does, and
//     documented as the run's one authoritative answer
func (r *_Run) _End(outcome error) {
	r.mutex.Lock()

	if r.outcome == nil {
		r.outcome = outcome
	}

	r.mutex.Unlock()

	r.stop()
}

// _Outcome returns what ended this run, or nil if nothing did.
//
// Revisions:
//   - 2026-09-20 00:12: initial creation
func (r *_Run) _Outcome() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.outcome
}

// _Number gives the next child of parent its id, under the lock that keeps
// two racing spawns from sharing one.
//
// The ordinal is per parent rather than per run. One counter shared by every
// thread numbers in the order spawns happen, which is a fact about time; an id
// that names its parent is a fact about structure, and the two disagree
// whenever a spawned function spawns before its siblings start.
//
// Revisions:
//   - 2026-09-19 20:36: initial creation, as _Track
//   - 2026-09-21 00:59: assigns an id naming its parent, counted per parent
//     rather than per run
//   - 2026-09-21 08:09: numbers only; nothing records the handle, since the
//     run's context is what cancels it
func (r *_Run) _Number(parent string) string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.ordinal[parent]++

	return _Child(parent, r.ordinal[parent])
}

// _Child is the id of a thread's nth child.
//
// The spine is the one special case: it contributes no prefix, so its children
// are thread_1 and thread_2 rather than thread_0_1.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func _Child(parent string, ordinal int32) string {
	if parent == SPINE {
		return fmt.Sprintf("%s%d", THREAD, ordinal)
	}

	return fmt.Sprintf("%s_%d", parent, ordinal)
}

// _Of returns the evaluation a thread belongs to.
//
// Revisions:
//   - 2026-09-19 20:36: initial creation
//   - 2026-09-21 08:09: returns the evaluation rather than the run, which it
//     carries
func _Of(thread *starlark.Thread) (*_Locals, error) {
	locals, ok := thread.Local(LOCALS_KEY).(*_Locals)
	if !ok {
		return nil, ErrNoRun
	}

	return locals, nil
}

// Outcome returns what ended the run on thread, or nil if nothing did.
//
// A caller asks this **after** ending the run, never before: ending waits for
// every thread the run started, and a thread still running is a thread that can
// still fail. Asked too early, this reports nothing and a run that had already
// been stopped looks like a success.
//
// Revisions:
//   - 2026-09-20 00:08: initial creation, as Cause
//   - 2026-09-20 00:12: renamed, and documented as something to ask last
func Outcome(thread *starlark.Thread) error {
	locals, err := _Of(thread)
	if err != nil {
		return nil
	}

	return locals.run._Outcome()
}

// Context returns the context of the evaluation on thread.
//
// A builtin that blocks needs it. Cancelling a run stops the interpreter
// between instructions, which does nothing to a Go call already waiting - so
// anything that waits must wait on this too, or a cancelled run hangs instead
// of ending. It is the evaluation's own context, not the run's: a bounded call
// is cancellable on its own, and something blocked inside it must observe that
// narrower cancel or the thing bounding it waits for the full duration anyway.
//
// Revisions:
//   - 2026-09-20 00:57: initial creation
//   - 2026-09-21 08:09: read from the one local
func Context(thread *starlark.Thread) (context.Context, error) {
	locals, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	return locals.ctx, nil
}

// Fail records cause as why the run on thread ended and stops the run - unless
// something is catching assertions, in which case it returns cause and stops
// nothing.
//
// The one place that question is asked. assert calls it, and so does a retry
// that has given up; asking it in one caller and not the other is how an
// exhausted inner retry used to end the run under an outer one.
//
// A failure nothing catches stops everything: the spine, every spawned thread,
// and anything they spawned. That is what a test runner does - the first
// assertion ends the test. The outcome is recorded before anything is
// cancelled, so the run's answer is already fixed by the time the shutdown
// starts producing errors of its own.
//
// Returns cause unchanged either way, so a caller raises it.
//
// Revisions:
//   - 2026-09-20 00:09: initial creation, as _Stop
//   - 2026-09-20 01:20: End added beside it, for retry
//   - 2026-09-21 08:09: one exported function replacing both, so nothing can
//     end a run without the catching question being asked
func Fail(thread *starlark.Thread, cause error) error {
	locals, err := _Of(thread)
	if err != nil || locals.catching > 0 {
		return cause
	}

	locals.run._End(cause)

	return cause
}

// AttemptOf is the attempt the evaluation on thread is counted as, or
// NO_ATTEMPT outside a repeat or a retry.
//
// Inherited: a thread spawned inside an attempt is inside that attempt, so an
// attempt's work can spawn and still ask which attempt it is.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func AttemptOf(thread *starlark.Thread) int32 {
	locals, err := _Of(thread)
	if err != nil {
		return NO_ATTEMPT
	}

	return locals.attempt
}

// Shared returns this run's value for key, building it the first time it is
// asked for.
//
// It is how a plugin holds something that belongs to one execution rather than
// to the process or to a compile - a store two spawned threads share, say. Two
// runs of one artifact get two values; the threads within a run get the same
// one, because they carry the same run.
//
// build is called at most once per run per key, under the lock, so two threads
// racing to be first still see one value.
//
// The map is keyed to any because what a plugin stores is its own business and
// no two plugins store the same shape. The type parameter is what keeps that
// contained: a caller names the type it expects and never sees the assertion.
//
// Returns ErrNoRun when the thread has no run, and ErrNotLocal when key already
// holds something of another type - which means two plugins chose one key.
//
// Revisions:
//   - 2026-09-20 00:31: initial creation, as Local
//   - 2026-09-21 09:46: renamed: what a run shares is not what an evaluation
//     carries, and both were called locals
func Shared[T any](thread *starlark.Thread, key string, build func() T) (T, error) {
	var empty T

	locals, err := _Of(thread)
	if err != nil {
		return empty, err
	}

	run := locals.run

	run.mutex.Lock()
	defer run.mutex.Unlock()

	held, found := run.shared[key]
	if !found {
		made := build()
		run.shared[key] = made

		return made, nil
	}

	value, ok := held.(T)
	if !ok {
		return empty, fmt.Errorf("%s holds a %T: %w", key, held, ErrNotLocal)
	}

	return value, nil
}
