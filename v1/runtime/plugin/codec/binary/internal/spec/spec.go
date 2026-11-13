// Package spec is internal, not public: the names, base and bit masks of the binary plugin.
//
// Revisions:
//   - 2026-11-08 00:59: initial creation (C91w12c, Q228, Q243)
package spec

// NAME is the module, and the four names it holds.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	NAME       = "binary"
	FROM_BYTES = "from_bytes"
	TO_BYTES   = "to_bytes"
	FROM_INT   = "from_int"
	TO_INT     = "to_int"

	// BITS_PER_BYTE is how many bits make a whole byte, and BASE what the bits
	// count in.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	BITS_PER_BYTE = 8
	BASE          = 2
)

// The masks below read a uint64 as eight bytes side by side, which is how eight
// bits become eight digits, and eight digits one byte, in a handful of
// operations rather than a loop of eight. A word holds eight characters in the
// order they are read: the first in its lowest byte.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	// LANES is 1 in every byte: a byte multiplied by it is a copy in each.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	LANES = 0x0101010101010101

	// ZEROS is the digit 0 in every byte. Added to eight 0s and 1s it makes
	// eight digits; taken from eight digits it leaves eight 0s and 1s.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	ZEROS = 0x3030303030303030

	// SHARED is every bit of a byte that the digits 0 and 1 share, which is all
	// but the lowest: eight characters are all digits exactly when they match
	// ZEROS on these.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	SHARED = 0xFEFEFEFEFEFEFEFE

	// SPREAD keeps one bit of each copy: the highest in the first byte, down to
	// the lowest in the last, so the digits come out most significant first.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	SPREAD = 0x0102040810204080

	// CARRY lifts whatever bit a byte kept into its highest. 0x7F and any one
	// bit add to at least 0x80 and at most 0xFF, so no byte carries into the
	// next.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	CARRY = 0x7F7F7F7F7F7F7F7F

	// GATHER sends the lowest bit of byte k to bit 63-k, so a product's top byte
	// holds eight 0s and 1s as one byte, the first most significant. Every other
	// term of the product lands above bit 63, or on a bit of its own below 56
	// where nothing adds up to carry into the top byte.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	GATHER = 0x8040201008040201

	// TOP_BYTE is where a word's top byte begins: shifted down this far, it
	// stands alone.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	TOP_BYTE = 56
)
