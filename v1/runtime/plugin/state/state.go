// Package state gives a script a store its threads can share. Importing it is
// what enables it.
//
// Module scope is frozen before anything concurrent runs, so a script cannot
// share data by assigning to a global. This is the way two spawned threads pass
// anything to each other.
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package state

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/store"
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
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _State struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 00:23: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (s *_State) Name() string {
	return spec.NAME
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
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 11:32: uses store.Get and store.Update, which moved to internal/store (C120b,
//     Q315)
func (s *_State) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.SET: starlark.NewBuiltin(
					spec.NAME+"."+spec.SET,
					_Set,
				),
				spec.GET: starlark.NewBuiltin(
					spec.NAME+"."+spec.GET,
					store.Get,
				),
				spec.UPDATE: starlark.NewBuiltin(
					spec.NAME+"."+spec.UPDATE,
					store.Update,
				),
			},
		},
	}
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
// Returns scheduler.ErrNested when called from inside an update, as a nested update is:
// waiting there would be waiting for a lock this evaluation already holds,
// which never ends.
//
// Revisions:
//   - 2026-09-20 00:25: initial creation
//   - 2026-09-24 16:08: takes the name's lock, so a set cannot be lost to an
//     update that started before it
//   - 2026-09-30 21:19: charges the name the first time it is used, as well as
//     the value
//   - 2026-11-01 13:00: charges neither, the budget being gone
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 11:32: uses store.SetArgs, store.SetData and store.SetKeep, which moved to
//     internal/store (C120b, Q315)
func _Set(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	name, value, err := store.SetArgs(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	err = store.SetData(
		thread,
		fn,
		name,
		value,
	)
	if err != nil {
		return nil, err
	}

	return store.SetKeep(
		thread,
		fn,
		name,
		value,
	)
}
