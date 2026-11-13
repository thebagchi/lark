// Package run is internal, not public: it holds the behavior of the scheduler,
// the run, its evaluations, handles and locks.
//
// Package run holds the behavior of the scheduler: the run, its evaluations,
// handles and locks. No host imports it; package scheduler forwards to it, and
// the types a host sees are the interfaces in package common.
//
// Revisions:
//   - 2026-11-07 19:15: initial creation, with Gate, GATE and the scheduler's constants
//     moved from scheduler (C91w4, Q227, Q228)
//   - 2026-11-07 19:59: holds the rest of the scheduler, moved from scheduler (C91w5, Q116)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
package run

import "sync"

// Gate is a lock only the scheduler can reach: the package is internal, so
// the exported variable GATE is no handle on it for a caller.
//
// Revisions:
//   - 2026-11-04 18:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:15: moved to package run as Gate, so scheduler can call
//     its methods (C91w4, Q227)
type Gate struct {
	mutex sync.Mutex
}

// Enter takes the gate.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54 fix round 1, B094)
//   - 2026-11-07 19:15: moved to package run as Enter (C91w4, Q227)
func (g *Gate) Enter() {
	g.mutex.Lock()
}

// Leave gives the gate back.
//
// Revisions:
//   - 2026-11-04 00:30: initial creation (C54 fix round 1, B094)
//   - 2026-11-07 19:15: moved to package run as Leave (C91w4, Q227)
func (g *Gate) Leave() {
	g.mutex.Unlock()
}

// GATE orders the changes Begin and the function it returns make to a thread:
// its locals, its print, its step hook and its step limit. A thread belongs to
// one goroutine, and Begin is called once per thread by Eval, so the gate is
// never contended in lark. It is there for a caller that shares one thread
// anyway. It is held for a few stores, never across a wait. The convention
// checker wants a package-level variable in CAPITAL_SNAKE, hence the exported
// name; the package is internal, so the name gives a host nothing.
//
// Revisions:
//   - 2026-11-04 18:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:15: moved to package run, which no host can import (C91w4, Q227)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
var (
	GATE Gate
)
