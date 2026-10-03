package graph

import (
	"strings"
)

// _Lines is a source, its lines, and the byte offset each line starts at.
//
// The parser gives a position as a line and a column that counts runes, and
// slicing the source there needs a byte offset. Walking the source from its
// start for each position cost the square of its length, and the cube for a
// body whose every line carries a comment, since each node's comments were
// walked to as well. With the line starts found once, a position is a lookup
// and a walk along its own line.
//
// One type for both directions: a derivation slices a function's body out of
// its module, and an emitter slices a node's text out of a parsed body.
type _Lines struct {
	src    string
	lines  []string
	starts []int
}

// _NewLines is src with its lines and their starts found.
//
// Revisions:
//   - 2026-10-03 20:39: initial creation
func _NewLines(src string) *_Lines {
	lines := strings.Split(src, NEWLINE)
	starts := make([]int, len(lines))
	at := 0

	for idx, line := range lines {
		starts[idx] = at
		at += len(line) + len(NEWLINE)
	}

	return &_Lines{src: src, lines: lines, starts: starts}
}

// _At is the byte offset of a one-based line and rune column, or MISSING when
// the text has no such place.
//
// The column after a line's last rune is a place, where a statement that ends
// the line ends; on the last line it is the end of the text.
//
// Revisions:
//   - 2026-10-03 20:39: initial creation, replacing a walk from the start of the
//     source in two places
func (l *_Lines) _At(line int32, col int32) int {
	if line < FIRST || col < FIRST || int(line) > len(l.lines) {
		return MISSING
	}

	start := l.starts[line-1]
	runes := int32(FIRST)

	for idx := range l.lines[line-1] {
		if runes == col {
			return start + idx
		}

		runes++
	}

	if runes == col {
		return start + len(l.lines[line-1])
	}

	return MISSING
}

// _LineEnd is the byte offset where a one-based line ends, before its
// newline.
//
// Revisions:
//   - 2026-10-03 20:39: initial creation, from a function of the same name that found
//     the line by walking the source
func (l *_Lines) _LineEnd(line int32) int {
	return l.starts[line-1] + len(l.lines[line-1])
}
