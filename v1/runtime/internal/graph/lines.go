package graph

import (
	"sync"
)

const (
	// LONG is the length in bytes above which a line keeps marks for At.
	//
	// Revisions:
	//   - 2026-11-12 23:43: initial creation (C130, B289, Q402)
	LONG = 256

	// STRIDE is the number of runes between two marks of a long line.
	//
	// Revisions:
	//   - 2026-11-12 23:43: initial creation (C130, B289, Q402)
	STRIDE = 64
)

// Lines is a source, its lines, and the byte offset each line starts at.
//
// The parser gives a position as a line and a column that counts runes, and
// slicing the source there needs a byte offset. Walking the source from its
// start for each position cost the square of its length, and the cube for a
// body whose every line carries a comment, since each node's comments were
// walked to as well. With the line starts found once, a position is a lookup
// and a walk along its own line.
//
// One type for every reader of positions: a derivation slices a function's body
// out of its module, an emitter slices a node's text out of a parsed body, and
// a merge finds where an edit goes. Make one with NewLines: the line starts are
// found the first time a method needs them, once, so a Lines may be shared by
// goroutines. A line longer than LONG bytes keeps marks, found the first time
// At asks for a place on it, under hold (B289).
//
// A line ends where the scanner of the parser ends one: at a line feed, and at
// a carriage return that no line feed follows. So the line of any position the
// parser gives is a line of this table.
//
// Revisions:
//   - 2026-10-03 20:39: initial creation, as _Lines
//   - 2026-11-05 23:24: exported, made by NewLines and read by amalgam, which kept a
//     copy of its own (C60, Q188)
//   - 2026-11-10 11:58: a lone carriage return ends a line, as the scanner reads it (C118, B260)
//   - 2026-11-12 23:43: holds the byte offset of every 64th rune of a long line (C130, B289, Q402)
type Lines struct {
	hold   sync.Mutex
	src    string
	once   sync.Once
	lines  []string
	starts []int
	marks  map[int32][]int
}

// NewLines is src with its lines to be found. Nothing is split until a method
// needs a place.
//
// Revisions:
//   - 2026-11-05 23:24: initial creation, the one way to make a Lines (C60, Q198)
func NewLines(src string) *Lines {
	return &Lines{
		src: src,
	}
}

// At is the byte offset of a one-based line and rune column, or MISSING when
// the text has no such place.
//
// The column after a line's last rune is a place, where a statement that ends
// the line ends; on the last line it is the end of the text.
//
// A line longer than LONG bytes keeps the offset of every STRIDE-th rune, found
// once, so a place is a jump and a walk of fewer than STRIDE runes, where it was
// a walk from the line start: a script of many statements on one line cost the
// square of its length (B289).
//
// Revisions:
//   - 2026-10-03 20:39: initial creation, replacing a walk from the start of the
//     source in two places
//   - 2026-11-05 23:24: exported, and finds the line starts on first use (C60,
//     Q188)
//   - 2026-11-12 23:43: starts the walk at a mark on a long line (C130, B289, Q402)
func (l *Lines) At(
	line int32,
	col int32,
) int {
	l._Find()

	if line < FIRST || col < FIRST || int(line) > len(l.lines) {
		return MISSING
	}

	var (
		start = l.starts[line-1]
		text  = l.lines[line-1]
		runes = int32(FIRST)
		from  = 0
	)

	if len(text) > LONG {
		var (
			marks = l._Marked(line)
			mark  = min(
				int(col-FIRST)/STRIDE,
				len(marks)-1,
			)
		)

		runes += int32(mark * STRIDE)
		from = marks[mark]
	}

	for idx := range text[from:] {
		if runes == col {
			return start + from + idx
		}

		runes++
	}

	if runes == col {
		return start + len(text)
	}

	return MISSING
}

// End is the byte offset where a one-based line ends, before its newline, or
// MISSING when the text has no such line.
//
// Revisions:
//   - 2026-10-03 20:39: initial creation, from a function of the same name that found
//     the line by walking the source
//   - 2026-11-05 23:24: exported, and MISSING for a line the text lacks (C60,
//     Q188)
func (l *Lines) End(line int32) int {
	l._Find()

	if line < FIRST || int(line) > len(l.lines) {
		return MISSING
	}

	return l.starts[line-1] + len(l.lines[line-1])
}

// _Marked is the byte offset of rune 1, STRIDE+1, 2*STRIDE+1 and so on of a
// one-based line, found the first time it is asked for. It is asked for a line
// longer than LONG bytes, which has runes, so the first mark is rune 1.
//
// Revisions:
//   - 2026-11-12 23:43: initial creation (C130, B289, Q402)
func (l *Lines) _Marked(line int32) []int {
	l.hold.Lock()
	defer l.hold.Unlock()

	if marks, found := l.marks[line]; found {
		return marks
	}

	var (
		marks []int
		runes = 0
	)

	for idx := range l.lines[line-1] {
		if runes%STRIDE == 0 {
			marks = append(
				marks,
				idx,
			)
		}

		runes++
	}

	if l.marks == nil {
		l.marks = map[int32][]int{}
	}

	l.marks[line] = marks

	return marks
}

// _Find splits src into lines and finds where each starts, the first time it
// runs.
//
// A line ends at a line feed, or at a carriage return that no line feed
// follows. That is where the scanner of the parser ends one, and the line of
// every position the parser gives counts them so (scan.go, readRune in the
// pinned go.starlark.net). A carriage return before a line feed stays on its
// line, as it always did. Every line end is one byte, so a start is the end of
// the line before it, plus one.
//
// Revisions:
//   - 2026-11-05 23:24: initial creation, from _NewLines (C60, Q188)
//   - 2026-11-10 11:58: a lone carriage return ends a line too (C118, B260)
func (l *Lines) _Find() {
	l.once.Do(func() {
		at := 0

		for at <= len(l.src) {
			end := _LineEnd(
				l.src,
				at,
			)

			l.starts = append(
				l.starts,
				at,
			)
			l.lines = append(
				l.lines,
				l.src[at:end],
			)

			at = end + 1
		}
	})
}

// _LineEnd is the byte offset where the line of src that starts at from ends:
// the line feed or the lone carriage return that ends it, or the end of src.
//
// Revisions:
//   - 2026-11-10 11:58: initial creation (C118, B260)
func _LineEnd(
	src string,
	from int,
) int {
	for end := from; end < len(src); end++ {
		switch src[end] {
		case '\n':
			return end

		case '\r':
			if end+1 == len(src) || src[end+1] != '\n' {
				return end
			}
		}
	}

	return len(src)
}

// Semicolon is the offset after the semicolon that joins a statement to the one
// after it, and true; or at, and false, when no semicolon does. at is the offset
// the parser gives as the end of the statement. An offset below zero finds none.
//
// go.starlark.net ends an index, a slice and a parenthesised tuple at the
// closing bracket, not after it (syntax.go, Span of IndexExpr, SliceExpr,
// TupleExpr), so at may stand on a bracket. Nothing but closing brackets,
// spaces, tabs and line continuations (a backslash before a line break) stands
// between the end of a statement and its semicolon, so they are skipped, and no
// rule by node kind is needed (B288, B291). A comment, a line break with no
// backslash or any other byte ends the search.
//
// The text is a string or bytes: the derivation holds a string, the merge bytes.
//
// Revisions:
//   - 2026-11-12 23:36: initial creation, shared by derivation and merge (C130, B288, B291, Q65)
func Semicolon[T ~string | ~[]byte](
	text T,
	at int,
) (int, bool) {
	if at < 0 {
		return at, false
	}

	for next := at; next < len(text); next++ {
		switch text[next] {
		case ' ', '\t', ']', ')':
			continue

		case '\\':
			if skipped := _Continued(
				text,
				next,
			); skipped > next {
				next = skipped

				continue
			}

		case ';':
			return next + 1, true
		}

		break
	}

	return at, false
}

// Following reports whether a statement starts after offset at, past spaces,
// tabs and line continuations: the next byte is none of a comment, a line break
// or the end of the text. at is the offset after a semicolon, so a semicolon at
// the end of a line, or before a comment, joins nothing (B288, Q400). An offset
// below zero finds none.
//
// Revisions:
//   - 2026-11-13 00:45: initial creation, shared by the derivation and the merge (C130, B285)
func Following[T ~string | ~[]byte](
	text T,
	at int,
) bool {
	if at < 0 {
		return false
	}

	for next := at; next < len(text); next++ {
		switch text[next] {
		case ' ', '\t':
			continue

		case '\\':
			if skipped := _Continued(
				text,
				next,
			); skipped > next {
				next = skipped

				continue
			}

		case '#', '\n', '\r':
			return false
		}

		return true
	}

	return false
}

// _Continued is the offset of the last byte of the line break a backslash at
// offset at starts, or at when the backslash is not before a line break. A line
// break is a line feed, a carriage return, or both.
//
// Revisions:
//   - 2026-11-12 23:36: initial creation, out of Semicolon (C130, B291)
func _Continued[T ~string | ~[]byte](
	text T,
	at int,
) int {
	after := at + 1
	if after >= len(text) {
		return at
	}

	switch text[after] {
	case '\n':
		return after

	case '\r':
		if after+1 < len(text) && text[after+1] == '\n' {
			return after + 1
		}

		return after
	}

	return at
}
