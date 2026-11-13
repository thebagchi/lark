package common

import (
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Plugin is a set of names a script is given.
//
// The interface is declared here rather than borrowed so that what a host
// depends on is the two things it asks: what a plugin is called, and what it
// contributes. The name is what makes a conflict reportable - without it a
// clash can only name the key, not who supplied it.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation, in package plugin
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:16: moved to package common (C91w1)
type Plugin interface {
	Name() string
	Values() starlark.StringDict
}

// Checking is a plugin that can refuse a use of its names by reading the
// source, before anything runs.
//
// Optional: a compiler asks every plugin that implements it and leaves the
// rest alone. That is what keeps the knowledge in the right place - a plugin
// knows what its own names mean, and a compiler that had to know would be a
// compiler coupled to every plugin anybody writes.
//
// A tree, not a string, because the question is about what was written rather
// than how it was spelled. Returning an error refuses the compile.
//
// Revisions:
//   - 2026-09-24 20:41: initial creation, in package plugin
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:16: moved to package common (C91w1)
type Checking interface {
	Check(tree *syntax.File) error
}
