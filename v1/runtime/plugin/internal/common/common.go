// Package common holds the small helpers that several plugins would otherwise
// write each for themselves: the refusal of a text, the check of a width, a
// builtin that checks its arguments first, the from_int of a codec, and the
// reading of an argument's source.
//
// It is internal to plugin: a plugin imports it, and no host and no script
// does. Its names are exported so that the plugins can reach them, and no
// further. It imports unpack, so unpack cannot import it.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package common
