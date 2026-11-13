package common

const (
	// BITS_PREFIX is what every reader of bits takes off before reading, in either
	// case.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to common from binary as BITS_PREFIX (C91w12c, Q244, Q246)
	BITS_PREFIX = "0b"
)

// QUOTE_LIMIT is the most bytes of a script's text that an error message quotes.
// Go's own encoding/json/v2 cuts the pointer it quotes to 100 bytes the same
// way (TruncatePointer in jsontext/errors.go).
//
// Revisions:
//   - 2026-11-02 11:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to common from unpack (C91w12c, Q244)
const (
	QUOTE_LIMIT = 100
)

// ANONYMOUS is the name an error gives a nil builtin, as core's is.
//
// Revisions:
//   - 2026-11-05 00:19: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to common from unpack (C91w12c, Q244)
const (
	ANONYMOUS = "builtin"
)
