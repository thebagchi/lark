package graph

import (
	"strings"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// COMMENT opens a line comment.
const COMMENT = "#"

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
func _Body(text *_Lines, def *syntax.DefStmt) string {
	if len(def.Body) == 0 {
		return ""
	}

	lines := text.lines

	first, _ := def.Body[0].Span()
	_, last := def.Body[len(def.Body)-1].Span()

	start := text._At(first.Line, first.Col)
	head := def.Rparen.Line

	// A body on the header's own line has no indentation of its own to remove,
	// and nothing above it under the header.
	if first.Line == head {
		return text.src[start:text._LineEnd(last.Line)]
	}

	indent := text.src[text._At(first.Line, FIRST):start]
	from := _First(lines, head, first.Line)
	to := _Last(lines, last.Line, indent)
	closers := _Closers(def.Body)

	var kept []string

	for line := from; line <= to; line++ {
		if closers[int32(line)] && strings.TrimSpace(lines[line-1]) == END {
			continue
		}

		kept = append(kept, lines[line-1])
	}

	return _Dedent(strings.Join(kept, "\n"), indent)
}

// _First is the first line of a body: the first comment line under the
// header, or the first statement's line when no comment comes before it.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation
func _First(lines []string, head int32, first int32) int {
	for line := head + 1; line < first; line++ {
		if strings.HasPrefix(strings.TrimSpace(lines[line-1]), COMMENT) {
			return int(line)
		}
	}

	return int(first)
}

// _Last is the last line of a body: the last statement's line, or the last
// comment line after it that is written at the body's indentation or further
// in. A blank line counts only when a comment of the body follows it.
//
// Revisions:
//   - 2026-10-02 00:00: initial creation
func _Last(lines []string, last int32, indent string) int {
	to := int(last)

	for line := int(last) + 1; line <= len(lines); line++ {
		text := lines[line-1]
		trimmed := strings.TrimSpace(text)

		if trimmed == "" {
			continue
		}

		inside := strings.HasPrefix(text, indent) && strings.HasPrefix(trimmed, COMMENT)
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

	_Closing(body, found)

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
func _Closing(body []syntax.Stmt, found map[int32]bool) {
	closed := len(dialect.Closed(body)) < len(body)
	if closed {
		found[syntax.Start(body[len(body)-1]).Line] = true
	}

	for _, stmt := range body {
		branch, ok := stmt.(*syntax.IfStmt)
		if ok {
			_Closing(branch.True, found)
			_Closing(branch.False, found)

			continue
		}

		suite := _SuiteOf(stmt)
		if suite != nil {
			_Closing(*suite, found)
		}
	}
}

// _Dedent removes indent from the front of every line that carries it.
//
// A blank line is left blank rather than filled with spaces. A line further
// out than indent - a comment written at the margin inside a def - is left as
// it is.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: takes the indentation itself rather than a count of
//     spaces
//   - 2026-10-02 00:00: dedents the first line too, since a body is whole
//     lines now
func _Dedent(body string, indent string) string {
	lines := strings.Split(body, "\n")

	for i := range lines {
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = ""

			continue
		}

		lines[i] = strings.TrimPrefix(lines[i], indent)
	}

	return strings.Join(lines, "\n")
}

// _Indent prefixes each line. A blank line stays blank, so a nested block
// does not gain trailing spaces.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
func _Indent(text, indent string) string {
	if text == "" {
		return ""
	}

	var out []string

	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")

			continue
		}

		out = append(out, indent+line)
	}

	return strings.Join(out, "\n")
}

// _Params is the names in a def's signature, in order, and whether every one
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
func _Params(def *syntax.DefStmt) ([]string, bool) {
	var names []string

	for _, param := range def.Params {
		name, ok := param.(*syntax.Ident)
		if !ok {
			return nil, false
		}

		names = append(names, name.Name)
	}

	return names, true
}
