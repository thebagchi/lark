package graph

import (
	"strings"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// COMMENT opens a line comment.
//
// Revisions:
//   - 2026-09-20 21:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	COMMENT = "#"
)

// _Body is the inside of a def as text: whole lines, from the first comment or
// statement under the header to the last line of the body, with the
// indentation the def gave them removed and every closing pass taken out.
//
// Whole lines, so a line comment is kept where it was written: one above the
// first statement, one between two, and one after the last statement at the
// body's own indentation, which is still inside the def. A comment written
// further out than the body belongs to whatever follows the def.
//
// The closer is the generator's, in both directions: a flow does not store it,
// and the generator writes it back. A pass that is the whole of a suite is not
// a closer, so a def that is only pass stores "pass".
//
// Sliced rather than printed because a body is text the script's author wrote,
// and it is stored as they wrote it.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: dedents by the first line's actual indentation
//   - 2026-10-02 00:00: takes whole lines, so line comments before the first
//     statement and after the last stay in the body, and drops every closing
//     pass rather than only the def's own
//   - 2026-10-03 20:40: takes its module as a text whose lines are already found, so
//     a module of many functions is not split again for each
//   - 2026-11-05 23:26: takes a *Lines, and cuts with its At and End in place of _At and
//     _LineEnd (C60, Q188)
//   - 2026-11-11 22:33: a body on the header's line joins its lines as _KeptLines does
//     (C122, B264)
func _Body(
	text *Lines,
	def *syntax.DefStmt,
) string {
	if len(def.Body) == 0 {
		return ""
	}

	var (
		first syntax.Position
		last  syntax.Position
		head  = def.Rparen.Line
	)

	first, _ = def.Body[0].Span()
	_, last = def.Body[len(def.Body)-1].Span()

	start := text.At(
		first.Line,
		first.Col,
	)

	// A body on the header's own line has no indentation of its own to remove,
	// and nothing above it under the header.
	if first.Line == head {
		return _Joined(
			text,
			first.Line,
			last.Line,
			start,
		)
	}

	return _KeptLines(
		&_Clip{
			text:  text,
			def:   def,
			first: first,
			last:  last,
			start: start,
		},
	)
}

// _Joined is the text from byte start, on line from, to the end of line to,
// with the lines joined by a line feed as _KeptLines joins them, so a body on
// the header's line and one under it are kept the same way, and a lone carriage
// return that ends a line inside a bracket is a line feed in both (B264).
//
// Revisions:
//   - 2026-11-11 22:33: initial creation, the slice _Body took (C122, B264)
func _Joined(
	text *Lines,
	from int32,
	to int32,
	start int,
) string {
	kept := []string{text.src[start:text.End(from)]}

	for line := from + 1; line <= to; line++ {
		kept = append(
			kept,
			text.lines[line-1],
		)
	}

	return strings.Join(
		kept,
		"\n",
	)
}

// _Clip is the span of a body that sits under its header.
//
// Revisions:
//   - 2026-11-05 23:24: the text field is a *Lines in place of a *_Lines (C60, Q188)
type _Clip struct {
	text  *Lines
	def   *syntax.DefStmt
	first syntax.Position
	last  syntax.Position
	start int
}

// _KeptLines is the body with the header's indentation removed.
//
// A line that only closes a nest the body itself opened is left out.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-05 23:24: finds the indent with Lines.At in place of _At (C60, Q188)
//   - 2026-11-10 01:00: now uses First, Last, were _First, _Last (C120a, Q314)
//   - 2026-11-12 18:20: keeps a line inside a string as it was, with its indent (C128, B282)
func _KeptLines(clip *_Clip) string {
	indent := clip.text.src[clip.text.At(
		clip.first.Line,
		FIRST,
	):clip.start]

	var (
		from = First(
			clip.text.lines,
			clip.def.Rparen.Line,
			clip.first.Line,
		)
		closers = _Closers(clip.def.Body)
		strung  = _Strung(clip.def)
		to      = Last(
			clip.text.lines,
			clip.last.Line,
			indent,
		)
	)

	var kept []string

	for line := from; line <= to; line++ {
		if closers[int32(line)] && strings.TrimSpace(clip.text.lines[line-1]) == END {
			continue
		}

		_, inside := strung[int32(line)]
		if inside {
			kept = append(
				kept,
				clip.text.lines[line-1],
			)

			continue
		}

		kept = append(
			kept,
			_Dedent(
				clip.text.lines[line-1],
				indent,
			),
		)
	}

	return strings.Join(
		kept,
		"\n",
	)
}

// First is the first line of a body: the first comment line under the
// header, or the first statement's line when no comment comes before it.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation
//   - 2026-11-08 10:35: skips a line outside the text (C108, B216)
//   - 2026-11-10 00:25: renamed from _First, as a test in tests/ needs it (C120a, Q314)
func First(
	lines []string,
	head int32,
	first int32,
) int {
	for line := head + 1; line < first; line++ {
		if line < 1 || int(line) > len(lines) {
			continue
		}

		if strings.HasPrefix(
			strings.TrimSpace(lines[line-1]),
			COMMENT,
		) {
			return int(line)
		}
	}

	return int(first)
}

// Last is the last line of a body: the last statement's line, or the last
// comment line after it that is written at the body's indentation or further
// in. A blank line counts only when a comment of the body follows it.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation
//   - 2026-11-08 10:35: a last line before the text is taken as none (C108, B217)
//   - 2026-11-10 00:25: renamed from _Last, as a test in tests/ needs it (C120a, Q314)
func Last(
	lines []string,
	last int32,
	indent string,
) int {
	to := max(
		int(last),
		0,
	)

	for line := to + 1; line <= len(lines); line++ {
		var text = lines[line-1]

		trimmed := strings.TrimSpace(text)

		if trimmed == "" {
			continue
		}

		inside := strings.HasPrefix(
			text,
			indent,
		) && strings.HasPrefix(
			trimmed,
			COMMENT,
		)
		if !inside {
			break
		}

		to = line
	}

	return to
}

// _Closers is the line of every closing pass in a body: a pass that ends a
// suite holding at least one other statement, in the def and in every suite
// inside it.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation
func _Closers(body []syntax.Stmt) map[int32]bool {
	found := make(map[int32]bool)

	_Closing(
		body,
		found,
	)

	return found
}

// _Closing records the closing pass of one suite, and of every suite its
// statements hold.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation
//   - 2026-10-02 01:14: finds the closer through dialect.Closed, which the
//     dialect reads a suite with too
//   - 2026-10-03 20:49: reaches a def's, a for's or a while's suite through _SuiteOf
func _Closing(
	body []syntax.Stmt,
	found map[int32]bool,
) {
	closed := len(dialect.Closed(body)) < len(body)
	if closed {
		found[syntax.Start(body[len(body)-1]).Line] = true
	}

	for _, stmt := range body {
		branch, ok := stmt.(*syntax.IfStmt)
		if ok {
			_Closing(
				branch.True,
				found,
			)
			_Closing(
				branch.False,
				found,
			)

			continue
		}

		suite := _SuiteOf(stmt)
		if suite != nil {
			_Closing(
				*suite,
				found,
			)
		}
	}
}

// _Dedent removes indent from the front of one line that carries it.
//
// A blank line is left blank rather than filled with spaces. A line further
// out than indent - a comment written at the margin inside a def - is left as
// it is. A line inside a string is not given to it, as its text is the
// string's value (B282).
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: takes the indentation itself rather than a count of
//     spaces
//   - 2026-10-02 00:00: dedents the first line too, since a body is whole
//     lines now
//   - 2026-11-12 18:20: takes one line in place of a body, so a caller can keep a string's
//     lines (C128, B282)
func _Dedent(
	line string,
	indent string,
) string {
	if strings.TrimSpace(line) == "" {
		return ""
	}

	return strings.TrimPrefix(
		line,
		indent,
	)
}

// _Strung is the lines that begin inside a string: for each string or bytes
// literal under node, the lines after the one it starts on, up to the one it
// ends on. The text of such a line is part of the string's value, so no indent
// may be taken from it or put on it (B282).
//
// A line ends where _LineEnd ends one, as the scanner of the parser does, so a
// literal that holds a lone carriage return counts it.
//
// Revisions:
//   - 2026-11-12 18:20: initial creation (C128, B282)
func _Strung(node syntax.Node) map[int32]struct{} {
	var found map[int32]struct{}

	syntax.Walk(
		node,
		func(item syntax.Node) bool {
			lit, ok := item.(*syntax.Literal)
			if !ok {
				return true
			}

			if lit.Token != syntax.STRING && lit.Token != syntax.BYTES {
				return true
			}

			line := lit.TokenPos.Line

			for at := 0; at < len(lit.Raw); {
				end := _LineEnd(
					lit.Raw,
					at,
				)
				if end >= len(lit.Raw) {
					break
				}

				line++

				if found == nil {
					found = map[int32]struct{}{}
				}

				found[line] = struct{}{}
				at = end + 1
			}

			return true
		},
	)

	return found
}

// _Indent prefixes each line, except those in keep, which are written as they
// were. A blank line stays blank, so a nested block does not gain trailing
// spaces.
//
// A line ends where Lines ends one, which _LineEnd finds: at a line feed, and
// at a carriage return that no line feed follows. The byte that ended a line is
// written back as it was, so the text keeps the breaks the scanner of the
// parser reads, and a line after a lone carriage return is indented too
// (B264). The result is made in one allocation.
//
// keep holds the numbers of lines of the wrapped text, which has the header of
// the def above the first line of text, so line n of text is line n+1 of it. A
// line inside a string is kept: its text is the value of the string (B282).
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-11-11 00:58: ends a line with _LineEnd, as Lines does, so a lone carriage return
//     ends one (C122, B264)
//   - 2026-11-12 18:20: leaves the lines in keep as they were (C128, B282)
func _Indent(
	text,
	indent string,
	keep map[int32]struct{},
) string {
	if text == "" {
		return ""
	}

	var out strings.Builder

	out.Grow(len(text) + len(indent)*(1+
		strings.Count(
			text,
			"\n",
		)+strings.Count(
		text,
		"\r",
	)))

	line := int32(1)

	for at := 0; at <= len(text); line++ {
		var (
			end = _LineEnd(
				text,
				at,
			)
			_, kept = keep[line+1]
		)

		if kept || strings.TrimSpace(text[at:end]) != "" {
			if !kept {
				out.WriteString(indent)
			}

			out.WriteString(text[at:end])
		}

		if end < len(text) {
			out.WriteByte(text[end])
		}

		at = end + 1
	}

	return out.String()
}

// Params is the names in a def's signature, in order, and whether every one
// of them is a plain name.
//
// A default, a *args or a **kwargs has nowhere to go: Function.params is a
// list of names. Carrying the bare name would change the program, because a
// call relying on the default would raise instead of defaulting - so a def
// with one is not modelled at all and keeps its body, where its signature
// still means what it says.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-11-08 10:35: a nil def has no names to carry (C108, B218)
//   - 2026-11-10 00:25: renamed from _Params, as a test in tests/ needs it (C120a, Q314)
func Params(def *syntax.DefStmt) ([]string, bool) {
	if def == nil {
		return nil, false
	}

	var names []string

	for _, param := range def.Params {
		name, ok := param.(*syntax.Ident)
		if !ok {
			return nil, false
		}

		names = append(
			names,
			name.Name,
		)
	}

	return names, true
}
