package common

import "go.starlark.net/starlark"

// BIG and LITTLE are the byte orders a script may name, BIG when it names
// none.
//
// They are a pair: a host that names one order names the other. Package buf
// reads the default from BIG.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:16: moved to package common, as a pair (Q124, C91w8b)
const (
	BIG    = "big"
	LITTLE = "little"
)

// Field is an integer, how many bytes it is written in, and their order.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:16: moved here from integer; data only (C91w8b)
type Field struct {
	Number starlark.Int
	Width  int
	Order  string
}
