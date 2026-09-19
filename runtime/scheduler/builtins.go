package scheduler

import "go.starlark.net/starlark"

// BUILTINS is what this plugin is called when a conflict has to name it.
const BUILTINS = "scheduler"

// Builtins is the runtime's own plugin: spawn, join, cancel and assert.
//
// It satisfies the plugin interface structurally - by having Name and Values,
// not by importing whoever declared it. That is deliberate and load-bearing:
// the artifact package imports this one, so this one cannot import the
// registry without a cycle. Something that imports both registers it.
//
// Empty: what it contributes is built on each call, so there is nothing to
// hold.
type Builtins struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-19 22:18: initial creation
func (b *Builtins) Name() string {
	return BUILTINS
}

// Values returns the four names the runtime itself supplies.
//
// A fresh map on every call, because a package-level variable would be one map
// shared by every host, and a host that added a name to it would be adding it
// to everybody's environment.
//
// Revisions:
//   - 2026-09-19 22:18: initial creation
func (b *Builtins) Values() starlark.StringDict {
	return starlark.StringDict{
		SPAWN:  starlark.NewBuiltin(SPAWN, _Spawn),
		JOIN:   starlark.NewBuiltin(JOIN, _Join),
		CANCEL: starlark.NewBuiltin(CANCEL, _Cancel),
		ASSERT: starlark.NewBuiltin(ASSERT, _Assert),
	}
}
