// Package scheduler runs a Starlark script's concurrent work: a spawned
// function gets a goroutine and an interpreter thread of its own, and a handle
// is what a script holds it by. The behavior lives in scheduler/internal/run;
// this package forwards to it, and the types it takes and returns are the
// interfaces of package common.
//
// # For plugin authors
//
// A builtin that blocks must wait on the context from Context(thread). Blocking
// means a wait, a sleep, or a network call. Cancelling a run does not stop a Go
// call that is already waiting. A builtin that ignores this context ends
// seconds late, after its own wait is over. A builtin that selects on
// Context(thread).Done() ends at once. ExampleContext shows the pattern. For
// real code, read the event, flow and core plugins. Core's sleep is the
// shortest.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:59: thin forwards to scheduler/internal/run (C91w5)
//   - 2026-11-08 02:09: section for plugin authors (C76, Q236)
package scheduler
