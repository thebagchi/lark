// Package dialect fixes the Starlark dialect every script in this runtime is
// compiled against, so two scripts are never two languages.
package dialect

import "go.starlark.net/syntax"

// OPTIONS is the dialect.
//
// Sets, while-loops and recursion are on: a test script reaches for all three,
// and a runtime that runs tests is not the place to withhold a loop. Recursion
// on means the interpreter stops checking for it, so a runaway recursive script
// is bounded by nothing here; whatever bounds a run - a step limit, a deadline -
// is a later decision and not this option's job.
//
// Global reassignment is off because a workflow freezes its globals before any
// concurrent call, so a script that rebinds one would fail at run time.
// Refusing it at compile time reports the same mistake with a position in it.
//
// Lambda is absent from this list because the library has no option for it -
// FileOptions covers these five fields and nothing else, and the resolver
// accepts a lambda under all of them. Refusing one everywhere would mean
// walking every parsed tree, which was considered on 2026-09-19 21:17 and not
// taken. A lambda is legal; spawn refuses one, so a spawned thread always has a
// name to report, and that is where the refusal earns its place.
var OPTIONS = &syntax.FileOptions{
	Set:             true,
	While:           true,
	TopLevelControl: true,
	GlobalReassign:  false,
	Recursion:       true,
}
