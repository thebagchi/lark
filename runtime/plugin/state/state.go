// Package state gives a script a store its threads can share. Importing it is
// what enables it.
//
// Module scope is frozen before anything concurrent runs, so a script cannot
// share data by assigning to a global. This is the way two spawned threads pass
// anything to each other.
package state

import (
	"errors"
	"fmt"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// ErrNested is returned when a thread starts an update while inside one.
var ErrNested = errors.New("state: an update cannot start another")

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

// _Store is one execution's store: what a script put there, the lock that makes
// it safe for threads to reach at once, and a lock per name for updates.
//
// keys holds one lock per name, taken for the whole of an update - read, call,
// write - which is what makes an update atomic. guard protects the two maps
// themselves and is never held while script code runs.
//
// busy records which name each thread is currently updating. A thread inside an
// update may not start another: two threads updating two names in opposite
// orders would deadlock, and there is no lock ordering a script could be asked
// to respect, because a script cannot see the locks at all.
type _Store struct {
	guard  sync.RWMutex
	values map[string]starlark.Value
	keys   map[string]*sync.Mutex
	busy   map[*starlark.Thread]string
}

// _Key returns the lock for name, making it the first time it is asked for.
//
// Revisions:
//   - 2026-09-20 00:38: initial creation
func (s *_Store) _Key(name string) *sync.Mutex {
	s.guard.Lock()
	defer s.guard.Unlock()

	held, found := s.keys[name]
	if !found {
		held = &sync.Mutex{}
		s.keys[name] = held
	}

	return held
}

// _Enter records that thread is updating name, or refuses when it is already
// updating something.
//
// Revisions:
//   - 2026-09-20 00:39: initial creation
func (s *_Store) _Enter(thread *starlark.Thread, name string) error {
	s.guard.Lock()
	defer s.guard.Unlock()

	inside, found := s.busy[thread]
	if found {
		return fmt.Errorf("already updating %q: %w", inside, ErrNested)
	}

	s.busy[thread] = name

	return nil
}

// _Leave records that thread has finished updating.
//
// Revisions:
//   - 2026-09-20 00:39: initial creation
func (s *_Store) _Leave(thread *starlark.Thread) {
	s.guard.Lock()
	defer s.guard.Unlock()

	delete(s.busy, thread)
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
	return scheduler.Local(thread, NAME, func() *_Store {
		return &_Store{
			values: map[string]starlark.Value{},
			keys:   map[string]*sync.Mutex{},
			busy:   map[*starlark.Thread]string{},
		}
	})
}

// _Set stores value under name and returns None.
//
// The value is frozen first. A store exists so that threads can reach it at
// once, and handing a mutable value to two threads is the race this package is
// meant to avoid - freezing turns a later mutation into a loud failure rather
// than corruption. It is also what Starlark does to module scope for the same
// reason.
//
// Revisions:
//   - 2026-09-20 00:25: initial creation
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

	value.Freeze()

	store.guard.Lock()
	defer store.guard.Unlock()

	store.values[name] = value

	return starlark.None, nil
}

// _Get returns what was last stored under name, or None if nothing was.
//
// None rather than an error, because a script reading a key another thread has
// not written yet is the ordinary case in a store threads share.
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
	defer store.guard.RUnlock()

	value, found := store.values[name]
	if !found {
		return starlark.None, nil
	}

	return value, nil
}

// _Update applies fn to what is stored under name and stores what it returns.
//
// It is the only safe way to change a value other threads are also changing.
// A get followed by a set is two operations, and another thread writing between
// them loses one of the updates - silently, and only under load. Measured
// before this existed: four threads incrementing one counter two hundred times
// each reached 294 of an expected 800.
//
// The lock is Go's and lasts for the whole of read, call and write. A script
// never sees it, and cannot forget to release it, because it never holds it.
//
// fn is called with the current value, or None when nothing is stored yet, and
// must return the new one. It runs while the name is locked, so it should do
// little: every other thread updating that name waits for it.
//
// Returns ErrNested when called from inside an update. Two threads updating two
// names in opposite orders would deadlock, and a script cannot be asked to take
// locks in an order it cannot see - so nesting is refused rather than ordered.
//
// Revisions:
//   - 2026-09-20 00:41: initial creation
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

	err = store._Enter(thread, name)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", fn.Name(), name, err)
	}

	defer store._Leave(thread)

	key := store._Key(name)

	key.Lock()
	defer key.Unlock()

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

	updated, err := starlark.Call(thread, change, starlark.Tuple{current}, nil)
	if err != nil {
		return nil, err
	}

	updated.Freeze()

	s.guard.Lock()
	defer s.guard.Unlock()

	s.values[name] = updated

	return updated, nil
}
