// Package buffer is internal, not public: it holds the buf module, Buffer,
// Reader and their methods.
//
// Package buffer holds the buf module: the Buffer and Reader values, the
// methods a script calls on them, and the module that carries both. Package
// buf registers it and re-exports its two errors.
//
// Revisions:
//   - 2026-11-07 19:17: initial creation, moved out of package buf (C91w8a)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
package buffer

import (
	"errors"
)

// ErrFrozen is returned for a change to a buffer or a reader that a spawn
// has frozen.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved from package buf, which now aliases it (C91w8a)
var (
	ErrFrozen = errors.New("frozen")

	// ErrShort is returned for a read of more bytes than a reader has left.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: moved from package buf, which now aliases it (C91w8a)
	ErrShort = errors.New("fewer bytes left than asked for")
)
