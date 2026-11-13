package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

var (
	// ErrRun is returned when a builtin is called on a thread no run set up,
	// and when a call is handed a nil object it needs: a nil callee, handle,
	// walk or receiver. The wrap names what is missing.
	//
	// Revisions:
	//   - 2026-09-19 23:36: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 17:15: renamed from ErrNoRun (C121, Q349)
	ErrRun = errors.New("no run or nothing to act on")

	// ErrLocal is returned when two plugins claim one key for values of
	// different types.
	//
	// Revisions:
	//   - 2026-09-20 00:24: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 17:15: renamed from ErrNotLocal (C121, Q349)
	ErrLocal = errors.New("a run local holds another type")
)

// Run is one entry point's execution.
//
// The context lives here rather than being passed, because a Starlark builtin
// is handed only a thread and a builtin is where spawn runs. go.md forbids a
// context on a struct that outlives the call it was scoped to; a Run is
// created by a call into an artifact and discarded when that call returns, so
// it does not.
//
// There is no list of live handles. Every evaluation's context descends from
// ctx, so cancelling ctx is what cancels everything, and a list of what to
// cancel would be a second copy of that fact - one that a spawn made during
// teardown was measured not to be on.
//
// outcome is the run's one answer, held by a pointer so that recording it is a
// single compare-and-swap rather than a lock: whoever swaps first wins, and
// nothing can swap it again. It is nil until something ends the run.
//
// count and shards spread the spawns that own their lane over a few contexts
// between ctx and the spawns, see _Shard. A shard is made on first use, so a
// run of a few spawns makes none.
//
// ordinal, shared, turns and locks are nil until the first spawn, the first
// shared value and the first lock: a run that does none of the three, which is
// most of them, makes no map. mutex guards the writes of those maps and nothing
// else, and is never held while a build runs. shared is the exception in how it
// is read: it points at a map that is never changed once published, so a reader
// loads it with no lock, and a writer under mutex publishes a copy (see Hold).
// turns holds one mutex for each shared key built, which orders the builds of
// that key.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: renamed from _Run and the fields ctx, group, shards, stop, as a test in
//     tests/ needs them (C120a, Q314)
//   - 2026-11-10 11:52: adds turns (C116, B238, Q296)
type Run struct {
	mutex   sync.Mutex
	Ctx     context.Context
	Stop    context.CancelFunc
	Group   sync.WaitGroup
	into    common.Reporter
	logger  *slog.Logger
	outcome atomic.Pointer[error]
	ordinal map[string]int32
	shared  atomic.Pointer[map[string]any]
	locks   map[string]chan struct{}
	turns   map[string]*sync.Mutex
	count   atomic.Uint32
	Shards  [16]atomic.Pointer[_Shard]
}

// _Shard is a context under the run's, which the spawns that hash to it derive
// from. Its stop is kept so the run lets go of it, and vet sees the cancel
// used.
//
// Revisions:
//   - 2026-11-05 18:53: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Shard struct {
	ctx  context.Context
	stop context.CancelFunc
}

// Parent is the context a spawn that owns its lane derives from.
//
// Every such spawn used to derive from ctx, so the spawner added a child and
// each ending thread removed one under the one mutex of ctx. This gives the
// first len(shards) spawns ctx itself, so a small run is as it was, and then
// spreads the rest over the shards, each made on first use under ctx. A shard
// is cancelled by ctx, so cancelling ctx still cancels every spawn, at one more
// level. A shard made after ctx was cancelled is cancelled, as a spawn made then
// always was.
//
// Revisions:
//   - 2026-11-05 08:20: initial creation (C53, Q67)
//   - 2026-11-10 00:25: renamed from _Parent, as a test in tests/ needs it (C120a, Q314)
func (r *Run) Parent() context.Context {
	var (
		n    = r.count.Add(1)
		size = uint32(len(r.Shards))
	)

	if n <= size {
		return r.Ctx
	}

	var (
		slot = &r.Shards[n%size]
		held = slot.Load()
	)
	if held != nil {
		return held.ctx
	}

	var (
		ctx, stop = context.WithCancel(r.Ctx)
		made      = &_Shard{
			ctx:  ctx,
			stop: stop,
		}
	)

	if slot.CompareAndSwap(
		nil,
		made,
	) {
		return ctx
	}

	stop()

	return slot.Load().ctx
}

// Release lets go of the shards, after the run's context was cancelled.
//
// Revisions:
//   - 2026-11-05 08:20: initial creation (C53, Q67)
//   - 2026-11-10 00:25: renamed from _Release, as a test in tests/ needs it (C120a, Q314)
func (r *Run) Release() {
	for idx := range r.Shards {
		held := r.Shards[idx].Load()
		if held != nil {
			held.stop()
		}
	}
}

// Locals is everything one evaluation carries: which run it belongs to,
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
//
// label and binding are what a spawn tells the reporter about the thread it
// starts: the function it runs, which a lambda cannot say for itself, and the
// name the script bound the spawn to. Set by Labelled, and never inherited.
//
// failed is the first failure Fail was handed on this evaluation, which the
// evaluation ends with however it returns. Never inherited: it belongs to the
// evaluation whose work failed.
//
// slot is the lock this evaluation holds, and nil when it holds none. issued
// counts the holds Lock has made, and hold is the number of the hold now held,
// or 0: the release Lock hands back carries its hold's number and acts only
// while that number is the one held, so a stale or repeated release does
// nothing. Neither is inherited, for the reason holding is not.
//
// Revisions:
//   - 2026-09-21 10:00: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: renamed from _Locals and the fields catching, ctx, hold, run, thread, as a
//     test in tests/ needs them (C120a, Q314)
type Locals struct {
	Run        *Run
	ThreadID   string
	Ctx        context.Context
	attempt    int32
	holding    bool
	Catching   int
	inside     string
	slot       chan struct{}
	HoldNumber atomic.Uint64
	issued     uint64
	label      string
	binding    string
	failed     error
}

// Begin attaches a new run to thread, with settings, and returns the function
// that ends it.
//
// This is the whole of what another package needs from this one: a call that
// wants spawn, join and cancel to work on its thread calls Begin, evaluates,
// and hands what it returns the evaluation's error. settings says what watches
// the run, where it prints and what it may hold; nil is a zero Settings.
//
// That error is how the spine's own line ends, by the rule Eval returns:
// the run's outcome when it has one, and otherwise the error, cancelled when
// the run was stopped. Without it a spine whose entry point raised would end
// as succeeded, since raising is not an outcome - nothing else ends the run.
// The function answers with what that rule made of it, which is what the run
// produced.
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
//   - 2026-10-02 00:38: reports the spine starting as a Line
//   - 2026-10-02 12:19: the function it returns takes the evaluation's error,
//     so the spine ends failed when its entry point raised
//   - 2026-10-02 13:12: print writes the transcript ctx carries, whether or not
//     anything reports
//   - 2026-10-02 16:21: print logs to the logger ctx carries, or to slog's
//     default
//   - 2026-10-03 08:31: takes the run's settings, rather than reading a reporter, a
//     logger and a ceiling off ctx
//   - 2026-10-03 20:53: the function it returns answers with what the run produced, so
//     the rule is applied once and its caller is handed the result
//   - 2026-11-01 13:00: settings carry no ceiling
//   - 2026-11-02 22:32: starts the memory watch when settings ask for one, and
//     stops it in the function it returns, after the wait (C27)
//   - 2026-11-04 06:55: changes the thread through _Seated, so two callers that share a thread
//     do not race, and a panic in it leaves the gate free (C54 fix rounds 1 and 2, B094)
//   - 2026-11-04 23:50: takes a nil context as Background and a nil thread as a new one, since a
//     function that returns only a finisher cannot refuse them (C86 fix round 3)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: now uses Begun, Ended, were _Begun, _Ended (C120a, Q314)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
func Begin(
	ctx context.Context,
	thread *starlark.Thread,
	name string,
	settings *common.Settings,
) func(err error) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if thread == nil {
		thread = new(starlark.Thread)
	}

	inner, stop := context.WithCancel(ctx)

	if settings == nil {
		settings = new(common.Settings)
	}

	var (
		run = Begun(
			inner,
			stop,
			settings,
		)
		watching = _Seated(
			inner,
			thread,
			run,
		)
	)

	run._Tell(func() {
		run.into.Started(
			common.SPINE,
			&common.Line{
				Name: name,
			},
		)
	})

	return Ended(
		ctx,
		&_Done{
			stop:     stop,
			run:      run,
			watching: watching,
			unwatch: memwatch.Start(
				inner,
				settings.Watch,
				run._End,
			),
			name: name,
		},
	)
}

// _Seated puts the run on the thread: its locals, its print and its step
// watch. The gate is given back by a defer, so a panic (a nil thread) cannot
// leave the process-wide gate held.
//
// Revisions:
//   - 2026-11-04 06:55: initial creation, out of Begin (C54 fix round 2)
//   - 2026-11-05 19:07: arms the watch itself, with no one-use _CancelOn (C58)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 01:00: now uses Run, Watch, Locals, Run, and 3 more, were _Run, _Watch, _Locals,
//     run, and 3 more (C120a, Q314)
func _Seated(
	inner context.Context,
	thread *starlark.Thread,
	run *Run,
) *Watch {
	GATE.Enter()
	defer GATE.Leave()

	thread.SetLocal(
		LOCALS_KEY,
		&Locals{
			Run:      run,
			ThreadID: common.SPINE,
			Ctx:      inner,
		},
	)
	thread.Print = run._Print()

	watch := new(Watch)

	watch.Arm(
		inner,
		thread,
	)

	return watch
}

// _Unseated releases the watch under the gate, which a defer gives back.
//
// Revisions:
//   - 2026-11-04 06:55: initial creation, out of the function Begin returns (C54 fix round 2)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-10 01:00: now uses Watch, Release, were _Watch, _Release (C120a, Q314)
func _Unseated(watching *Watch) {
	GATE.Enter()
	defer GATE.Leave()

	watching.Release()
}

// Begun is the run a thread is about to carry.
//
// It makes none of the run's maps, which Run tells why.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 01:24: leaves the ordinal, shared and lock maps nil, for
//     whatever first writes each to make
//   - 2026-11-01 13:00: makes no budget
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: renamed from _Begun, as a test in tests/ needs it (C120a, Q314)
func Begun(
	inner context.Context,
	stop context.CancelFunc,
	settings *common.Settings,
) *Run {
	return &Run{
		Ctx:    inner,
		Stop:   stop,
		into:   settings.Reporter,
		logger: Logging(settings),
	}
}

// _Done is what the function Begin returns needs in order to finish the run.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:00: now uses Run, Watch, were _Run, _Watch (C120a, Q314)
type _Done struct {
	stop     context.CancelFunc
	run      *Run
	watching *Watch
	unwatch  func()
	name     string
}

// Ended is the function Begin returns. It waits, then reports the outcome.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 22:32: stops the memory watch after the wait, so a run stuck in
//     a builtin still meets the watch's grace (C27)
//   - 2026-11-04 00:30: releases the watch under GATE (C54 fix round 1, B094)
//   - 2026-11-04 06:55: releases it through _Unseated, which cannot hold the gate
//     on a panic (C54 fix round 2)
//   - 2026-11-05 08:20: lets go of the shards after the wait (C53, Q67)
//   - 2026-11-06 00:02: a nil done gives a function that ends nothing and answers with the error
//     (C100a, B167)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-10 00:25: renamed from _Ended, as a test in tests/ needs it (C120a, Q314)
func Ended(
	ctx context.Context,
	done *_Done,
) func(err error) error {
	if done == nil {
		return func(err error) error {
			return Produced(
				ctx,
				nil,
				err,
			)
		}
	}

	return func(err error) error {
		_Unseated(done.watching)

		done.stop()
		done.run.Group.Wait()
		done.run.Release()
		done.unwatch()

		produced := Produced(
			ctx,
			done.run.Outcome(),
			err,
		)

		done.run._Tell(func() {
			done.run.into.Ended(
				common.SPINE,
				done.name,
				produced,
			)
		})

		return produced
	}
}

// Outcome returns what ended this run, or nil if nothing did.
//
// Revisions:
//   - 2026-09-20 00:12: initial creation
//   - 2026-11-02 10:56: loads the outcome rather than reading it under the
//     run's mutex
//   - 2026-11-06 00:02: a nil run has no outcome (C100a, B168)
//   - 2026-11-10 00:25: renamed from _Outcome, as a test in tests/ needs it (C120a, Q314)
func (r *Run) Outcome() error {
	if r == nil {
		return nil
	}

	held := r.outcome.Load()
	if held == nil {
		return nil
	}

	return *held
}

// _Print is what a thread of this run hands the interpreter for print: a record
// of the run's logger, with the lane and the function that printed it.
//
// Guarded as a report is, because the logger is a host's, running on a thread
// of this runtime's: a handler that panics ends the run, rather than the
// process.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-09-21 09:46: tells the reporter, on the printing thread's lane
//   - 2026-10-02 13:12: writes the run's transcript, with the function that
//     printed, rather than telling the reporter
//   - 2026-10-02 16:21: logs a record, whose handler writes the time and the
//     layout
//   - 2026-11-02 11:56: polls the thread's context when the record is written, as a
//     builtin does when it returns
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:08: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-10 01:00: now uses Run, Ctx, were _Run, ctx (C120a, Q314)
//   - 2026-11-10 12:15: uses Lane, was _Lane (C116, B237, Q328)
func (r *Run) _Print() func(
	*starlark.Thread,
	string,
) {
	return func(
		thread *starlark.Thread,
		msg string,
	) {
		blown := guard.Contained(func() {
			r.logger.LogAttrs(
				r.Ctx,
				slog.LevelInfo,
				msg,
				slog.String(
					common.ATTR_THREAD,
					Lane(thread),
				),
				slog.String(
					common.ATTR_FUNCTION,
					_Caller(thread),
				),
			)
		})
		if blown != nil {
			r._End(fmt.Errorf(
				"%s: %w",
				LOGGER,
				blown,
			))
		}

		Poll(thread)
	}
}

// _Tell runs one report, and ends the run if the reporter raises.
//
// A reporter is a host's code running on a thread of this runtime's. Left
// unguarded here it leaves the run by a path the run does not recover, and for
// a caller that runs an artifact directly it takes the process down.
//
// It ends the run rather than being absorbed. A run whose report was never made
// has not been observed, and reporting success for it tells a host that
// everything worked including the part that did not.
//
// Revisions:
//   - 2026-09-20 12:00: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:08: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-10 01:00: now uses Run, were _Run (C120a, Q314)
func (r *Run) _Tell(report func()) {
	if r.into == nil {
		return
	}

	blown := guard.Contained(report)
	if blown != nil {
		r._End(fmt.Errorf(
			"%s: %w",
			REPORTER,
			blown,
		))
	}
}

// _End records the run's outcome and stops everything it started.
//
// A run has one outcome, and it is whatever was recorded first. Everything that
// happens afterwards - a spinning sibling cancelled, a thread told to stop
// between instructions, an evaluation unwinding - is the shutdown, not the
// reason for it.
//
// "First" is the compare-and-swap from nil that succeeds: the memory model
// totally orders the atomic operations on one variable, so exactly one swap
// finds nil. The load before it only spares the others an allocation. A nil
// outcome records nothing, so a later error still becomes the outcome, as it
// did when a nil left the field nil.
//
// Revisions:
//   - 2026-09-20 00:07: initial creation, as _Fail
//   - 2026-09-20 00:12: named for what it records rather than what it does, and
//     documented as the run's one authoritative answer
//   - 2026-11-02 10:56: records the outcome with a compare-and-swap rather
//     than under the run's mutex
//   - 2026-11-10 01:00: now uses Run, Stop, were _Run, stop (C120a, Q314)
func (r *Run) _End(outcome error) {
	if outcome != nil && r.outcome.Load() == nil {
		held := outcome

		r.outcome.CompareAndSwap(
			nil,
			&held,
		)
	}

	r.Stop()
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
//   - 2026-10-29 01:24: makes the ordinal map on the run's first spawn
//   - 2026-11-05 19:07: names the child with spelling.Child, with no one-use _Child (C58)
//   - 2026-11-10 01:00: now uses Run, were _Run (C120a, Q314)
func (r *Run) _Number(parent string) string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.ordinal == nil {
		r.ordinal = make(map[string]int32)
	}

	r.ordinal[parent]++

	return spelling.Child(
		parent,
		int(r.ordinal[parent]),
	)
}

// _Held returns the shared value for key, with no lock.
//
// It reads the published map, which nothing changes after publishing, so it
// needs no mutex and cannot race with Hold.
//
// Revisions:
//   - 2026-11-02 21:01: initial creation, for change C39a
//   - 2026-11-10 01:00: now uses Hold, Run, were _Hold, _Run (C120a, Q314)
func (r *Run) _Held(key string) (any, bool) {
	held := r.shared.Load()
	if held == nil {
		return nil, false
	}

	value, found := (*held)[key]

	return value, found
}

// Hold returns the shared value for key, calling build and publishing the
// result when no thread has yet.
//
// Each key has its own mutex, so the callers racing to be first for one key
// take turns and build runs once. The run mutex is not held while build runs:
// build may ask for another key, start a thread, or take as long as it needs,
// and only a caller of the same key waits for it. Under the key's mutex a
// caller looks again, since the thread ahead of it may have published key.
//
// A build that asks for its own key, directly or through other keys that ask
// for it, waits for itself. That is a fault of the build, and no wait here can
// end it.
//
// A build that panics publishes nothing and leaves the key's mutex free, so
// the next caller builds again.
//
// Publishing copies the map and adds the key, then stores the copy, under the
// run mutex. A run holds a few keys (one per plugin that keeps state), so the
// copy is small and made once per key per run.
//
// Revisions:
//   - 2026-11-02 21:01: initial creation, for change C39a
//   - 2026-11-10 01:00: now uses Hold, Run, were _Hold, _Run (C120a, Q314)
//   - 2026-11-10 11:52: builds outside the run mutex, under one mutex for each key (C116,
//     B238, Q296)
func (r *Run) _Hold(
	key string,
	build func() any,
) any {
	turn := r._Turn(key)

	turn.Lock()
	defer turn.Unlock()

	value, found := r._Held(key)
	if found {
		return value
	}

	made := build()

	r._Publish(
		key,
		made,
	)

	return made
}

// _Turn returns the mutex that orders the builds of key, making it the first
// time it is asked for. The mutex stays for the run: one for each key built.
//
// Revisions:
//   - 2026-11-10 11:52: initial creation (C116, B238, Q296)
func (r *Run) _Turn(key string) *sync.Mutex {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.turns == nil {
		r.turns = make(map[string]*sync.Mutex)
	}

	turn, found := r.turns[key]
	if !found {
		turn = new(sync.Mutex)
		r.turns[key] = turn
	}

	return turn
}

// _Publish adds key to the shared map by storing a copy of it, under the run
// mutex so that two keys published together both stay.
//
// Revisions:
//   - 2026-11-10 11:52: initial creation, from the body of _Hold (C116, B238, Q296)
func (r *Run) _Publish(
	key string,
	made any,
) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	next := make(map[string]any)

	if held := r.shared.Load(); held != nil {
		maps.Copy(
			next,
			*held,
		)
	}

	next[key] = made
	r.shared.Store(&next)
}

// Of returns the evaluation a thread belongs to.
//
// Revisions:
//   - 2026-09-19 20:36: initial creation
//   - 2026-09-21 08:09: returns the evaluation rather than the run, which it
//     carries
//   - 2026-11-03 21:57: a nil thread carries no run, so it is ErrNoRun and not a panic
//     (C52, B057 to B059, B067)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-10 00:25: renamed from _Of, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func Of(thread *starlark.Thread) (*Locals, error) {
	if thread == nil {
		return nil, ErrRun
	}

	locals, ok := thread.Local(LOCALS_KEY).(*Locals)
	if !ok {
		return nil, ErrRun
	}

	return locals, nil
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
//   - 2026-11-10 01:00: now uses Of, Ctx, were _Of, ctx (C120a, Q314)
func Context(thread *starlark.Thread) (context.Context, error) {
	locals, err := Of(thread)
	if err != nil {
		return nil, err
	}

	return locals.Ctx, nil
}

// Ambient returns the context of the evaluation on thread, or
// context.Background() when thread runs outside a run.
//
// For a walk that only has to stop when the run is cancelled: it has no error
// to give a thread that no run owns, and a Background context is one nothing
// cancels. A builtin that waits uses Context, which says when there is none.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func Ambient(thread *starlark.Thread) context.Context {
	ctx, err := Context(thread)
	if err != nil {
		return context.Background()
	}

	return ctx
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
//   - 2026-10-03 23:52: keeps cause on the evaluation as well, which then ends with
//     it, for a caller with nowhere to raise it
//   - 2026-11-02 12:28: ends the run through _Fail, which the end of a thread
//     shares (C26)
//   - 2026-11-03 21:57: a nil thread carries no run, so cause is returned and nothing
//     stops (C52, B059)
//   - 2026-11-10 01:00: now uses Of, Fail, were _Of, _Fail (C120a, Q314)
func Fail(
	thread *starlark.Thread,
	cause error,
) error {
	locals, err := Of(thread)
	if err != nil {
		return cause
	}

	return locals.Fail(cause)
}

// Fail is Fail for an evaluation whose locals the caller already holds.
//
// Shared by Fail, which assert calls, and _Eval, which calls it for an
// error that wraps trail.ErrFatal. So a value nested too deep ends the run the
// way an assertion does, retry catches it the way it catches one, and no
// builtin needs to call the scheduler to say so.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation, out of Fail (C26)
//   - 2026-11-06 00:02: ends no run for locals with none, and does nothing for nil locals
//     (C100a, B169)
//   - 2026-11-10 00:25: renamed from _Fail, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 23:25: uses _Eval, formerly _Evaluate (C121, Q350)
func (l *Locals) Fail(cause error) error {
	if l == nil {
		return cause
	}

	// Kept whether or not anything catches, for a caller with nowhere to raise
	// cause: a loop over a file ends quietly, Starlark's iterator having no
	// error to return. The evaluation ends with it all the same, so the
	// function whose work failed reads as failed, and an attempt that failed
	// this way fails rather than passing.
	if l.failed == nil {
		l.failed = cause
	}

	if l.Catching > 0 {
		return cause
	}

	if l.Run != nil {
		l.Run._End(cause)
	}

	return cause
}

// AttemptOf is the attempt the evaluation on thread is counted as, or
// common.NO_ATTEMPT outside a repeat or a retry.
//
// Inherited: a thread spawned inside an attempt is inside that attempt, so an
// attempt's work can spawn and still ask which attempt it is.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 01:00: now uses Of, were _Of (C120a, Q314)
func AttemptOf(thread *starlark.Thread) int32 {
	locals, err := Of(thread)
	if err != nil {
		return common.NO_ATTEMPT
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
// build is called at most once per run per key, so two threads racing to be
// first still see one value. It runs with the run's mutex released, under one
// mutex for that key: build may call Shared for another key or start a thread,
// and only another caller of the same key waits for it. A build that asks for
// its own key, directly or through others, waits for itself.
//
// The map is keyed to any because what a plugin stores is its own business and
// no two plugins store the same shape. The type parameter is what keeps that
// contained: a caller names the type it expects and never sees the assertion.
//
// Returns ErrRun when the thread has no run, and ErrLocal when key already
// holds something of another type - which means two plugins chose one key.
//
// Revisions:
//   - 2026-09-20 00:31: initial creation, as Local
//   - 2026-09-21 09:46: renamed: what a run shares is not what an evaluation
//     carries, and both were called locals
//   - 2026-10-29 01:05: finds the evaluation through EvaluationOf, and keeps
//     its body in SharedIn
//   - 2026-11-10 11:52: build runs outside the run mutex, once for each key (C116, B238, Q296)
//   - 2026-11-10 17:15: uses ErrRun and ErrLocal, formerly ErrNoRun and ErrNotLocal (C121, Q349)
func Shared[T any](
	thread *starlark.Thread,
	key string,
	build func() T,
) (T, error) {
	evaluation, err := EvaluationOf(thread)
	if err != nil {
		var empty T

		return empty, err
	}

	return SharedIn(
		evaluation,
		key,
		build,
	)
}
