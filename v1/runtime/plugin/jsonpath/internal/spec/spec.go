// Package spec is internal, not public: the names the jsonpath module, JSON
// pointers and patch operations spell.
//
// Revisions:
//   - 2026-11-08 00:26: initial creation (C91w12b)
package spec

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-20 00:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:26: moved here from plugin/jsonpath (C91w12b)
const (
	NAME     = "jsonpath"
	PATCH    = "patch_json"
	FIND     = "find_key"
	EXTRACT  = "extract_json"
	MATCH    = "match_json"
	LENGTH   = "len_json"
	NO_VALUE = 0
)

// OP is the field a patch names its operation with, and the names that follow are the operations.
//
// Revisions:
//   - 2026-09-20 00:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:26: moved here from plugin/jsonpath (C91w12b)
const (
	OP      = "op"
	PATH    = "path"
	VALUE   = "value"
	FROM    = "from"
	ADD     = "add"
	REMOVE  = "remove"
	REPLACE = "replace"
	MOVE    = "move"
	COPY    = "copy"
	TEST    = "test"
)

// SEPARATOR splits a JSON pointer, and the names that follow are its escape codes.
//
// Revisions:
//   - 2026-09-20 00:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:26: moved here from plugin/jsonpath (C91w12b)
const (
	SEPARATOR = "/"
	APPEND    = "-"
	ESCAPED_1 = "~1"
	LITERAL_1 = "/"
	ESCAPED_0 = "~0"
	LITERAL_0 = "~"
)
