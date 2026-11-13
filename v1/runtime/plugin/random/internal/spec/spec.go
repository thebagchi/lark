// Package spec is internal, not public: the fixed words and limits of the random plugin.
//
// Revisions:
//   - 2026-11-08 00:27: initial creation (C91w12a)
package spec

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
const (
	NAME    = "random"
	SEED    = "seed"
	INT     = "int"
	FLOAT   = "float"
	CHOICE  = "choice"
	SHUFFLE = "shuffle"
	BYTES   = "bytes"

	// BYTE is how many values a byte has, which is the range each one is drawn
	// from.
	//
	// Revisions:
	//   - 2026-09-26 23:55: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
	BYTE = 256
)
