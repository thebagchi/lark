// Package store holds the keep side of the state plugin: the store of a run, its
// entries, and the get, set, update and keep builtins that read and write them.
// The plugin type, its registration and the set builtin of a flow stay in
// package state.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package store

import (
	"fmt"
	"sync"
	"sync/atomic"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Store is one execution's store: every name a script has used, and the lock
// that makes the map safe for threads to reach at once.
//
// The lock per name that makes an update atomic is the scheduler's, because
// it has to wait on the evaluation's context and refuse nesting through any
// thread the update started - both of which only the scheduler can see.
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed the fields held to Held,
//     for the tests in tests/ (C120b, Q315, Q314)
type _Store struct {
	guard sync.RWMutex
	Held  map[string]*Entry
}

// Entry is one name in the store: the value last kept under it.
//
// value is read by a get without the name's lock, which only a set or an update
// takes, so it is an atomic pointer to the value (Q212). Nil means nothing was
// kept, which reads as None. The store's own lock, above, still guards the map
// of entries.
//
// Revisions:
//   - 2026-09-30 22:37: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Entry to Entry; renamed
//     the fields value to Value, for the tests in tests/ (C120b, Q315, Q314)
type Entry struct {
	Value atomic.Pointer[starlark.Value]
}

// Of returns the store belonging to the run of evaluation, making it the first
// time this execution asks.
//
// Revisions:
//   - 2026-09-20 00:34: initial creation
//   - 2026-09-30 21:19: one map of entries rather than a map of values and a
//     map of charges
//   - 2026-10-29 01:05: takes the evaluation the builtin found, rather than
//     finding it on the thread again
//   - 2026-11-07 20:07: takes common.Evaluation in place of *scheduler.Evaluation (C91w5, Q127)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Of to Of, for the tests
//     in tests/ (C120b, Q315, Q314)
func Of(evaluation common.Evaluation) (*_Store, error) {
	return scheduler.SharedIn(
		evaluation,
		spec.NAME,
		func() *_Store {
			return &_Store{
				Held: map[string]*Entry{},
			}
		},
	)
}

// _Named is the entry for name, made the first time the run uses it.
//
// A new entry holds None, which is what a get of a name nothing has written
// answers anyway.
//
// Revisions:
//   - 2026-09-30 21:19: initial creation
//   - 2026-11-01 13:00: charges nothing and cannot fail, the budget being gone
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state (C120b, Q315)
func (s *_Store) _Named(name string) *Entry {
	s.guard.Lock()
	defer s.guard.Unlock()

	held, found := s.Held[name]
	if found {
		return held
	}

	held = new(Entry)
	s.Held[name] = held

	return held
}

// SetArgs reads the name and the value a set was given.
//
// Through unpack rather than starlark.UnpackPositionalArgs, which moved the
// name and the value to the heap: two allocations on every set. The refusals
// have the same text.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 01:32: reads the arguments through unpack, so they stay on
//     the stack
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _SetArgs to SetArgs, for
//     the tests in tests/ (C120b, Q315, Q314)
func SetArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, starlark.Value, error) {
	const NEEDED = 2

	err := unpack.Positional(
		fn,
		args,
		kwargs,
		NEEDED,
	)
	if err != nil {
		return "", nil, err
	}

	name, err := unpack.StringAt(
		fn,
		args,
		0,
	)
	if err != nil {
		return "", nil, err
	}

	return name, args[1], nil
}

// SetData refuses a value the store would not keep, and stops the run.
//
// The check is before the lock. What the value is does not depend on who
// holds the name, and a doomed call should not wait out an update.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 12:28: a value nested past common.DEPTH is an error that ends
//     the run (C26)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _SetData to SetData, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func SetData(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	name string,
	value starlark.Value,
) error {
	bad, ok, err := deep.IsData(
		scheduler.Ambient(thread),
		value,
	)
	if err != nil {
		return fmt.Errorf(
			"%s %s: %w",
			shared.Called(fn),
			unpack.QuotePrefix(name),
			err,
		)
	}

	if ok {
		return nil
	}

	return scheduler.Fail(
		thread,
		fmt.Errorf(
			"%s %s: %s: not data a store can hold: %w",
			shared.Called(fn),
			unpack.QuotePrefix(name),
			bad.Type(),
			unpack.ErrConvert,
		),
	)
}

// SetKeep stores value under name, holding the lock.
//
// The evaluation is found on the thread once, and the store and the lock both
// come from it. Each find is a lookup on the thread, and a set
// made three. It is found here, after the value is checked, so a set of
// something that is not data says so on a thread with no run, as it did.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:56: reads the budget once, for the name and for the value
//   - 2026-10-29 01:05: finds the evaluation once, for the store, the budget
//     and the lock
//   - 2026-11-01 13:00: takes the name's entry without a charge
//   - 2026-11-06 00:08: a nil builtin is scheduler.ErrNoCallee (C100a, B174)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _SetKeep to SetKeep, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func SetKeep(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	name string,
	value starlark.Value,
) (starlark.Value, error) {
	if fn == nil {
		return nil, fmt.Errorf(
			"nothing to call: %w",
			scheduler.ErrRun,
		)
	}

	evaluation, err := scheduler.EvaluationOf(thread)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	store, err := Of(evaluation)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return Kept(
		evaluation,
		&Keep{
			Fn:    fn,
			Held:  store._Named(name),
			name:  name,
			Value: value,
		},
	)
}

// Keep is one value about to be stored under its name.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Keep to Keep; renamed
//     the fields fn to Fn, held to Held, value to Value, for the tests in tests/ (C120b, Q315,
//     Q314)
type Keep struct {
	Fn    *starlark.Builtin
	Held  *Entry
	name  string
	Value starlark.Value
}

// Kept holds the name and stores the value, locking through evaluation.
//
// evaluation is a parameter and not a field of keep. The value in keep goes
// into the store, so the compiler moves keep's contents to the heap, and it
// does not tell one field from another. As a field, the evaluation was an
// allocation on every set.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:56: charges the budget the keep carries, rather than
//     reading it from the thread again
//   - 2026-10-29 01:05: locks and charges through the evaluation it is
//     handed, and takes no thread
//   - 2026-11-01 13:00: charges nothing, so the keep cannot fail
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-05 22:40: releases through the Hold that Lock returns (C58 fix 2, Q201)
//   - 2026-11-06 00:08: a keep with no builtin or no entry is scheduler.ErrNoCallee, and one
//     with no
//     value is ErrNotData (C100a, B177)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 20:07: takes common.Evaluation in place of *scheduler.Evaluation; a nil one is
//     ErrNoRun (C91w5, Q127)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Kept to Kept, for the
//     tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun and ErrConvert, formerly ErrNoRun and ErrNotProtoValue (C121,
//     Q349)
func Kept(
	evaluation common.Evaluation,
	keep *Keep,
) (starlark.Value, error) {
	if keep == nil || keep.Fn == nil || keep.Held == nil {
		return nil, fmt.Errorf(
			"nothing to call: %w",
			scheduler.ErrRun,
		)
	}

	if keep.Value == nil {
		return nil, fmt.Errorf(
			"%s %s: not data a store can hold: %w",
			shared.Called(keep.Fn),
			unpack.QuotePrefix(keep.name),
			unpack.ErrConvert,
		)
	}

	if evaluation == nil {
		return nil, scheduler.ErrRun
	}

	hold, err := evaluation.Lock(keep.name)
	if err != nil {
		return nil, fmt.Errorf(
			"%s %s: %w",
			shared.Called(keep.Fn),
			unpack.QuotePrefix(keep.name),
			err,
		)
	}

	defer scheduler.Release(hold)

	keep.Value.Freeze()

	keep.Held.Keep(keep.Value)

	return starlark.None, nil
}

// Get returns a copy of what was last stored under name, or None if nothing
// was.
//
// A copy, not the value itself. What is stored is frozen so that threads
// reading one name at once cannot be handed something another can change
// underneath them - and a script that cannot change what it read back cannot
// build the next value from it. So the store keeps the frozen original and
// hands out something the caller owns.
//
// This is read-copy-update: read the published version, change your own copy,
// publish the result with set or update. Nothing a script does to a copy is
// visible anywhere until it is stored.
//
// The copy costs time proportional to the size of the value, on every read. A
// script walking a large structure should read it once rather than in a loop.
//
// None rather than an error for a name nothing has written, because a script
// reading a key another thread has not written yet is the ordinary case in a
// store threads share. For the same reason a get makes no entry: a wait on an
// event has to make the thing it waits on, and a read has nothing to wait on.
//
// Revisions:
//   - 2026-09-20 00:26: initial creation
//   - 2026-09-30 21:19: reads the name's entry, and makes none
//   - 2026-10-29 01:05: finds the store through the evaluation, as a set does
//   - 2026-10-29 01:32: reads the name through _GetArgs, so it stays on the
//     stack
//   - 2026-11-02 12:28: the copy takes the thread's context (C26)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Get to Get, for the
//     tests in tests/ (C120b, Q315, Q314)
func Get(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	name, err := GetArgs(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	evaluation, err := scheduler.EvaluationOf(thread)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	store, err := Of(evaluation)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	store.guard.RLock()
	held, found := store.Held[name]
	store.guard.RUnlock()

	if !found {
		return starlark.None, nil
	}

	copied, err := deep.Copy(
		scheduler.Ambient(thread),
		held.Load(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return copied, nil
}

// GetArgs reads the name a get was given.
//
// Through unpack, for the reason SetArgs gives: one allocation on every get.
//
// Revisions:
//   - 2026-10-29 01:32: initial creation
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _GetArgs to GetArgs, for
//     the tests in tests/ (C120b, Q315, Q314)
func GetArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, error) {
	err := unpack.Positional(
		fn,
		args,
		kwargs,
		1,
	)
	if err != nil {
		return "", err
	}

	name, err := unpack.StringAt(
		fn,
		args,
		0,
	)
	if err != nil {
		return "", err
	}

	return name, nil
}

// Update applies fn to what is stored under name and stores what it returns.
//
// It is the only safe way to change a value other threads are also changing.
// A get followed by a set is two operations, and another thread writing between
// them loses one of the updates - silently, and only under load. Measured
// before this existed: four threads incrementing one counter two hundred times
// each reached 294 of an expected 800.
//
// The lock is the scheduler's and lasts for the whole of read, call and write.
// A script never sees it, and cannot forget to release it, because it never
// holds it. It waits on the evaluation's context, so a run that ends while a
// thread is waiting for a name ends rather than hangs.
//
// fn is called with the current value, or None when nothing is stored yet, and
// must return the new one. It runs while the name is locked, so it should do
// little: every other thread updating that name waits for it.
//
// Returns scheduler.ErrNested when called from inside an update, on this
// thread or on any thread that update started. Two threads updating two names
// in opposite orders would deadlock, and a script cannot be asked to take
// locks in an order it cannot see - so nesting is refused rather than ordered.
//
// Revisions:
//   - 2026-09-20 00:41: initial creation
//   - 2026-09-21 08:09: takes the scheduler's lock, which is cancellable and
//     refuses nesting wherever it happens
//   - 2026-09-30 21:19: charges the name the first time it is used, as a set
//     does
//   - 2026-10-29 00:56: reads the budget once, for the name and for what fn
//     returns
//   - 2026-10-29 01:05: finds the evaluation once, for the store, the budget
//     and the lock
//   - 2026-11-01 13:00: takes the name's entry without a charge
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Update to Update, for
//     the tests in tests/ (C120b, Q315, Q314)
func Update(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	name, change, err := UpdateArgs(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	evaluation, err := scheduler.EvaluationOf(thread)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	store, err := Of(evaluation)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return Updated(
		thread,
		evaluation,
		&Change{
			fn:     fn,
			Name:   name,
			Held:   store._Named(name),
			Change: change,
		},
	)
}

// Change is one function about to run under its name's lock.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Change to Change;
//     renamed the fields name to Name, held to Held, change to Change, for the tests in tests/
//     (C120b, Q315, Q314)
type Change struct {
	fn     *starlark.Builtin
	Name   string
	Held   *Entry
	Change starlark.Callable
}

// UpdateArgs reads the name and the function an update was given.
//
// Through unpack, for the reason SetArgs gives. The name is read before the
// function, so a call wrong in both is told about the name, as it was.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 01:32: reads the arguments through unpack, so they stay on
//     the stack
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _UpdateArgs to
//     UpdateArgs, for the tests in tests/ (C120b, Q315, Q314)
func UpdateArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, starlark.Callable, error) {
	const NEEDED = 2

	err := unpack.Positional(
		fn,
		args,
		kwargs,
		NEEDED,
	)
	if err != nil {
		return "", nil, err
	}

	name, err := unpack.StringAt(
		fn,
		args,
		0,
	)
	if err != nil {
		return "", nil, err
	}

	change, err := unpack.CallableAt(
		fn,
		args,
		1,
	)
	if err != nil {
		return "", nil, err
	}

	return name, change, nil
}

// Updated holds the name and applies the change, locking through
// evaluation, which is a parameter and not a field for the reason Kept gives.
//
// evaluation is the one the update was called in. The change runs on the
// same thread and may start attempts of its own, which put their evaluation
// on the thread while they run and put this one back when they end. So
// evaluation is still the update's when the change returns and its result is
// kept.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:56: hands the change's budget to the entry
//   - 2026-10-29 01:05: locks and charges through the evaluation it is
//     handed
//   - 2026-11-01 13:00: charges nothing
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-05 22:40: releases through the Hold that Lock returns (C58 fix 2, Q201)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 20:07: takes common.Evaluation in place of *scheduler.Evaluation; a nil one is
//     ErrNoRun (C91w5, Q127)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Updated to Updated, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun (C121, Q349)
func Updated(
	thread *starlark.Thread,
	evaluation common.Evaluation,
	change *Change,
) (starlark.Value, error) {
	if evaluation == nil {
		return nil, scheduler.ErrRun
	}

	hold, err := evaluation.Lock(change.Name)
	if err != nil {
		return nil, fmt.Errorf(
			"%s %s: %w",
			shared.Called(change.fn),
			unpack.QuotePrefix(change.Name),
			err,
		)
	}

	defer scheduler.Release(hold)

	updated, err := change.Held.Apply(
		thread,
		change.Name,
		change.Change,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s %s: %w",
			shared.Called(change.fn),
			unpack.QuotePrefix(change.Name),
			err,
		)
	}

	return updated, nil
}

// Apply calls change with what this entry holds, and keeps the result. name is
// for the message a refusal carries.
//
// The value is read and written atomically, so no lock is held around the
// call: script code runs for as long as it likes, and every get of this name
// would wait for it.
//
// change is handed a copy, for the same reason get hands one out: it has to be
// able to build the next value from the current one. The name's lock is held
// throughout, so nothing else publishes between the copy and the store - which
// is the whole difference between this and a get followed by a set.
//
// Revisions:
//   - 2026-09-20 00:43: initial creation
//   - 2026-09-30 21:19: the entry's rather than the store's, so a name nothing
//     has kept reads as the None its entry was made with
//   - 2026-10-29 00:56: charges the budget it is handed, rather than reading
//     it from the thread
//   - 2026-11-01 13:00: charges nothing
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 12:28: a value nested past common.DEPTH is an error that ends
//     the run (C26)
//   - 2026-11-06 00:08: a nil change is scheduler.ErrNoCallee (C100a, B172)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Apply to Apply, for the
//     tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRun and ErrConvert, formerly ErrNoRun and ErrNotProtoValue (C121,
//     Q349)
func (e *Entry) Apply(
	thread *starlark.Thread,
	name string,
	change starlark.Callable,
) (starlark.Value, error) {
	if change == nil {
		return nil, fmt.Errorf(
			"nothing to call: %w",
			scheduler.ErrRun,
		)
	}

	current, err := deep.Copy(
		scheduler.Ambient(thread),
		e.Load(),
	)
	if err != nil {
		return nil, err
	}

	updated, err := starlark.Call(
		thread,
		change,
		starlark.Tuple{current},
		nil,
	)
	if err != nil {
		return nil, err
	}

	// What the function returned is stored, so it answers to the same rule a
	// set does. Nothing the source could have shown refuses this one: what a
	// function returns is known when it returns.
	bad, ok, err := deep.IsData(
		scheduler.Ambient(thread),
		updated,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s.%s %s: %w",
			spec.NAME,
			spec.UPDATE,
			unpack.QuotePrefix(name),
			err,
		)
	}

	if !ok {
		return nil, scheduler.Fail(
			thread,
			fmt.Errorf(
				"%s.%s %s: %s: not data a store can hold: %w",
				spec.NAME,
				spec.UPDATE,
				unpack.QuotePrefix(name),
				bad.Type(),
				unpack.ErrConvert,
			),
		)
	}

	updated.Freeze()

	e.Keep(updated)

	return updated, nil
}

// Load is what this entry holds: the value last kept, or None.
//
// Revisions:
//   - 2026-09-30 21:19: initial creation
//   - 2026-11-06 00:45: loads the atomic pointer, and an entry nothing was kept in is None
//     (C100a, B178, Q212)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Value to Load, for the
//     tests in tests/ (C120b, Q315, Q314)
func (e *Entry) Load() starlark.Value {
	held := e.Value.Load()
	if held == nil {
		return starlark.None
	}

	return *held
}

// Keep puts value in this entry.
//
// A store keeps what a script gave it for the life of the run: there is no
// delete, so a name once set is held until the run ends.
//
// Revisions:
//   - 2026-09-27 01:26: initial creation
//   - 2026-09-30 21:19: the entry's, charged to the budget it is handed rather
//     than looking one up
//   - 2026-11-01 13:00: charges nothing and cannot fail, the budget being gone
//   - 2026-11-06 00:45: stores through the atomic pointer, with no lock (C100a, B178, Q212)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/state and renamed _Keep to Keep, for the
//     tests in tests/ (C120b, Q315, Q314)
func (e *Entry) Keep(value starlark.Value) {
	e.Value.Store(&value)
}
