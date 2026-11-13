// Package spec is internal, not public: the names, group size and alphabet of the base32 plugin.
//
// Revisions:
//   - 2026-11-08 00:59: initial creation (C91w12c, Q228, Q243)
package spec

// NAME is the module, and the two names it holds.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	NAME   = "base32"
	ENCODE = "encode"
	DECODE = "decode"

	// GROUP is how many characters one group of the encoding holds, which the
	// padding of RFC 4648 section 6 fills.
	//
	// Revisions:
	//   - 2026-11-02 16:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	GROUP = 8

	// ALPHABET is the base32 alphabet of RFC 4648 section 6, in value order.
	//
	// Revisions:
	//   - 2026-11-02 16:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

	// BITS_PER_CHARACTER and BITS_PER_OCTET give the pad bits of a final group.
	// A group of r characters holds 5r bits, 5r%8 of them pad. A character holds
	// 5 bits, so a group whose pad is 5 or more has a character with no data in
	// it: that is a final group of 1, 3 or 6 characters, which RFC 4648 section 6
	// does not list.
	//
	// Revisions:
	//   - 2026-11-02 16:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	BITS_PER_CHARACTER = 5
	BITS_PER_OCTET     = 8
)
