// Package spec is internal, not public: the fixed words and names of the path plugin.
//
// Revisions:
//   - 2026-11-08 00:27: initial creation (C91w12a)
package spec

import (
	"path/filepath"
)

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:27: moved to internal/spec (C91w12a)
const (
	NAME      = "path"
	JOIN      = "join"
	DIR       = "dir"
	BASE      = "base"
	EXT       = "ext"
	STEM      = "stem"
	CLEAN     = "clean"
	SPLIT     = "split"
	PARTS     = "parts"
	ABS       = "isabs"
	SEPARATOR = string(filepath.Separator)
)
