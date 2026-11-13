// Package spec is internal, not public: the padding words of the unpack helpers.
//
// Revisions:
//   - 2026-11-08 00:59: initial creation (C91w12c, Q228, Q243)
package spec

// OPTIONAL ends the name of a parameter that a call may leave out, as it does
// in the names that starlark.UnpackArgs takes. Every parameter after the first
// one that ends with it is optional too.
//
// Revisions:
//   - 2026-11-04 06:52: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	OPTIONAL = "?"
)

// ZERO is the digit that Padded pads with, in every base. It is the one pad
// digit of the codec plugins that write digits.
//
// Revisions:
//   - 2026-11-02 17:41: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
const (
	ZERO = '0'
)

const (
	// PADDING is the pad character of RFC 4648 sections 4 and 6.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	PADDING = "="

	// LINE_BREAKS holds CR and LF, which the standard library's decoders skip.
	//
	// Revisions:
	//   - 2026-11-02 16:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:59: moved to internal/spec (C91w12c, Q228)
	LINE_BREAKS = "\r\n"
)
