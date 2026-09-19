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

	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/scheduler"
)

const (
	NAME = "state"
	SET  = "set"
	GET  = "get"
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
				SET: starlark.NewBuiltin(NAME+"."+SET, _Set),
				GET: starlark.NewBuiltin(NAME+"."+GET, _Get),
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
