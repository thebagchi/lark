// Package spec is internal, not public: the panic text guard raises for a nil err pointer.
//
// Revisions:
//   - 2026-11-08 00:52: initial creation, from guard (Q243, C91w12d)
package spec

// NIL_ERR is the panic text for a nil err pointer.
//
// Revisions:
//   - 2026-11-03 06:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: moved here from guard (Q243, C91w12d)
const (
	NIL_ERR = "guard.WithRecover: nil err pointer"
)
