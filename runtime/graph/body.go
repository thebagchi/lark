package graph

import (
	"strings"

	"go.starlark.net/syntax"
)

// _Body is the source of a function's statements, with the indentation its def
// gave them removed.
//
// A Graph carries a body as statements at column zero and the generator puts
// the def's level back, so taking the text as written would leave every line
// but the first carrying four spaces it should not have - which reads as a
// body whose second line is nested inside its first.
//
// Sliced rather than printed because go.starlark.net parses and does not
// unparse: there is nothing to print an AST back with, and writing one would
// mean owning the formatting of a language whose whitespace is syntax.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: dedents by the first line's actual indentation
func _Body(src string, def *syntax.DefStmt) string {
	body := _Trimmed(def.Body)
	if len(body) == 0 {
		return ""
	}

	first, _ := body[0].Span()
	_, last := body[len(body)-1].Span()

	from := _Offset(src, first.Line, first.Col)
	to := _Offset(src, last.Line, last.Col)

	if from >= to || to > len(src) {
		return ""
	}

	// The whitespace the first statement sits behind, whatever it is made
	// of, is what every later line loses. Counting columns instead treated a
	// tab as one space and left tab-indented bodies indented.
	indent := src[_Offset(src, first.Line, 1):from]

	return _Dedent(src[from:to], indent)
}

// _Trimmed is a body without the pass the generator closes every def with.
//
// A body sliced out of generated source carries that pass, and emitting it
// again would write a second one under the first. The close is punctuation the
// generator owns, in both directions: the walk reads one without complaint and
// this drops one rather than carrying it as content.
//
// A body that is nothing but a pass trims to nothing, which is right - the
// generator writes the close back.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Trimmed(body []syntax.Stmt) []syntax.Stmt {
	if len(body) == 0 {
		return nil
	}

	last, ok := body[len(body)-1].(*syntax.BranchStmt)
	if ok && last.Token == syntax.PASS {
		return body[:len(body)-1]
	}

	return body
}

// _Dedent removes indent from the front of every line but the first, which
// has already lost it to where its span began.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: takes the indentation itself rather than a count of
//     spaces
func _Dedent(body string, indent string) string {
	lines := strings.Split(body, "\n")

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = ""

			continue
		}

		lines[i] = strings.TrimPrefix(lines[i], indent)
	}

	return strings.Join(lines, "\n")
}

// _Offset turns a one-based line and rune column into a byte offset.
//
// A syntax.Position carries no byte offset and its column counts runes, so a
// script with any character outside ASCII would be sliced in the wrong place
// by treating the column as bytes.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Offset(src string, line int32, col int32) int {
	lines := strings.SplitAfter(src, "\n")

	at := 0

	for i := int32(0); i < line-1 && int(i) < len(lines); i++ {
		at += len(lines[i])
	}

	if int(line-1) >= len(lines) {
		return len(src)
	}

	runes := int32(0)

	for i := range lines[line-1] {
		if runes == col-1 {
			return at + i
		}

		runes++
	}

	return at + len(lines[line-1])
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
