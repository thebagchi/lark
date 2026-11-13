// Package spec is internal, not public: the fixed words and names of the json plugin.
//
// Revisions:
//   - 2026-11-08 00:27: initial creation (C91w12a)
package spec

// NAME is the module, and DECODE is the member that checks its text.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
const (
	NAME          = "json"
	DECODE        = "decode"
	ENCODE        = "encode"
	ENCODE_INDENT = "encode_indent"
)
