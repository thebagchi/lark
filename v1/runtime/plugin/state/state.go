// Package state gives a script a store its threads can share. Importing it is
// what enables it.
//
// Module scope is frozen before anything concurrent runs, so a script cannot
// share data by assigning to a global. This is the way two spawned threads pass
// anything to each other.
package state

import (
	"fmt"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	NAME   = "state"
	SET    = "set"
	GET    = "get"
	UPDATE = "update"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-20 00:23: initial creation
func init() {
	plugin.Register(&_State{})
}

// _State is the plugin. Empty: a store belongs to an execution, not to the
// plugin, so there is nothing here to hold.
type _State struct{}

// _Store is one execution's store: what a script put there, and the lock that
// makes it safe for threads to reach at once.
//
// The lock per name that makes an update atomic is the scheduler's, because
// it has to wait on the evaluation's context and refuse nesting through any
// thread the update started - both of which only the scheduler can see.
type _Store struct {
	guard  sync.RWMutex
	values map[string]starlark.Value
}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 00:23: initial creation
func (s *_State) Name() string {
	return NAME
}

// Values returns the state module.
//
// The builtins hold no store. They find one on the thread they are called from,
// which is what scopes it to an execution rather than to a compile or to the
// process: two runs of one artifact get two stores, and every thread inside a
// run gets the same one, because they carry the same run.
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-09-20 00:33: the store is per execution, found on the thread, rather
//     than one made here and closed over
func (s *_State) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				SET:    starlark.NewBuiltin(NAME+"."+SET, _Set),
				GET:    starlark.NewBuiltin(NAME+"."+GET, _Get),
				UPDATE: starlark.NewBuiltin(NAME+"."+UPDATE, _Update),
			},
		},
	}
}

// _Of returns the store belonging to the run on thread, making it the first
// time this execution asks.
//
// Revisions:
//   - 2026-09-20 00:34: initial creation
func _Of(thread *starlark.Thread) (*_Store, error) {
	return scheduler.Shared(thread, NAME, func() *_Store {
		return &_Store{
			values: map[string]starlark.Value{},
		}
	})
}

// _Set stores value under name and returns None.
//
// Storing something that is not data stops the whole run, the way a failed
// assertion does, rather than only the thread that did it. A thread nobody
// joins fails silently - its error reaches the report and never becomes the
// run's result - so a spawned worker storing a function would otherwise put
// nothing in the store and say nothing about it. The mistake is in the script
// rather than in the data, and a script author wants to hear about it.
//
// The value is frozen first. A store exists so that threads can reach it at
// once, and handing a mutable value to two threads is the race this package is
// meant to avoid - freezing turns a later mutation into a loud failure rather
// than corruption. It is also what Starlark does to module scope for the same
// reason.
//
// It takes the name's lock, which update holds across its read, its call and
// its write. Without it a set landing while an update's function was running
// was overwritten by what that update had read before the set happened - so
// the write that finished second lost, silently, and a script could not tell.
// The store's own mutex could not prevent that: it is held for the read and
// for the write, and released around the call, which is the window.
//
// Returns ErrNested when called from inside an update, as a nested update is:
// waiting there would be waiting for a lock this evaluation already holds,
// which never ends.
//
// Revisions:
//   - 2026-09-20 00:25: initial creation
//   - 2026-09-24 16:08: takes the name's lock, so a set cannot be lost to an
//     update that started before it
func _Set(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name  string
		value starlark.Value
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &name, &value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	store, err := _Of(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	// Before the lock, because what the value is does not depend on who holds
	// the name. Taken after, a doomed call would first wait out whatever
	// update is running - and a run cancelled during that wait would report
	// the cancellation rather than the mistake in the script.
	bad, ok := deep.IsData(value)
	if !ok {
		return nil, scheduler.Fail(thread, fmt.Errorf(
			"%s %q: %s: %w", fn.Name(), name, bad.Type(), ErrNotData))
	}

	release, err := scheduler.Lock(thread, name)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", fn.Name(), name, err)
	}

	defer release()

	value.Freeze()

	store.guard.Lock()
	defer store.guard.Unlock()

	store.values[name] = value

	return starlark.None, nil
}

// _Get returns a copy of what was last stored under name, or None if nothing
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
// store threads share.
//
// Revisions:
//   - 2026-09-20 00:26: initial creation
func _Get(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	store, err := _Of(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	store.guard.RLock()
	value, found := store.values[name]
	store.guard.RUnlock()

	if !found {
		return starlark.None, nil
	}

	copied, err := deep.Copy(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return copied, nil
}

// _Update applies fn to what is stored under name and stores what it returns.
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
func _Update(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name   string
		change starlark.Callable
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &name, &change)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	store, err := _Of(thread)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	release, err := scheduler.Lock(thread, name)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", fn.Name(), name, err)
	}

	defer release()

	updated, err := store._Apply(thread, name, change)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", fn.Name(), name, err)
	}

	return updated, nil
}

// _Apply reads name, calls change with it, and stores the result.
//
// The store's own lock is taken for the read and for the write, and released
// around the call: holding it while script code runs would block every other
// name as well as this one.
//
// change is handed a copy, for the same reason get hands one out: it has to be
// able to build the next value from the current one. The name's lock is held
// throughout, so nothing else publishes between the copy and the store - which
// is the whole difference between this and a get followed by a set.
//
// Revisions:
//   - 2026-09-20 00:43: initial creation
func (s *_Store) _Apply(
	thread *starlark.Thread,
	name string,
	change starlark.Callable,
) (starlark.Value, error) {
	s.guard.RLock()
	current, found := s.values[name]
	s.guard.RUnlock()

	if !found {
		current = starlark.None
	}

	current, err := deep.Copy(current)
	if err != nil {
		return nil, err
	}

	updated, err := starlark.Call(thread, change, starlark.Tuple{current}, nil)
	if err != nil {
		return nil, err
	}

	// What the function returned is stored, so it answers to the same rule a
	// set does. Nothing the source could have shown refuses this one: what a
	// function returns is known when it returns.
	bad, ok := deep.IsData(updated)
	if !ok {
		return nil, scheduler.Fail(thread, fmt.Errorf(
			"%s.%s %q: %s: %w", NAME, UPDATE, name, bad.Type(), ErrNotData))
	}

	updated.Freeze()

	s.guard.Lock()
	defer s.guard.Unlock()

	s.values[name] = updated

	return updated, nil
}
