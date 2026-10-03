package graph

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// _Program is the file as a syntax tree, plus what that tree does not store.
//
// elif is the if nodes written with elif. IfStmt records the keyword only as
// a source position, and a node built from the flow has no source, so the
// word is remembered here. sources is the text a parsed node was cut from,
// keyed by the filename the parser stored on its positions, and cuts is the
// end-of-line comments in that text, which are trimmed wherever it is copied.
//
// notes, elses and tails are where each line comment of a body goes: above a
// statement or an elif, above an else, or at the end of the suite whose first
// statement keys it. The parser hands each comment to the next node in the
// file whatever its indentation, so this is worked out once, by line and
// column, rather than read off the node.
type _Program struct {
	functions []*syntax.DefStmt
	constants []syntax.Stmt
	arguments []syntax.Stmt
	main      *syntax.DefStmt
	elif      map[*syntax.IfStmt]struct{}
	sources   map[string]*_Lines
	cuts      map[string][]syntax.Comment
	notes     map[syntax.Node][]syntax.Comment
	elses     map[*syntax.IfStmt][]syntax.Comment
	tails     map[syntax.Stmt][]syntax.Comment
	seq       int
}

// _File is a filename for one parsed body, distinct from every other body in
// the file. Positions remember it, and the source is stored under it.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func (p *_Program) _File(name string) string {
	p.seq++

	return fmt.Sprintf("%s-%d.star", name, p.seq)
}

// _Close appends pass to every suite that does not return and does not
// already end in pass. An elif is the false list holding one if, and that
// list is not itself a suite.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Close(prog *_Program) {
	for _, def := range prog.functions {
		_CloseDef(def, prog)
	}

	_CloseDef(prog.main, prog)
}

// _CloseDef closes a function and the suites inside it.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _CloseDef(def *syntax.DefStmt, prog *_Program) {
	if def == nil {
		return
	}

	_CloseBody(&def.Body, prog)
}

// _CloseBody closes each statement, then the list itself when it needs a
// closer.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _CloseBody(body *[]syntax.Stmt, prog *_Program) {
	for _, stmt := range *body {
		_CloseStmt(stmt, prog)
	}

	if _Ended(*body) {
		return
	}

	*body = append(*body, _Pass())
}

// _CloseStmt closes the suites one statement contains.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-03 20:49: reaches a def's, a for's or a while's suite through _SuiteOf
func _CloseStmt(stmt syntax.Stmt, prog *_Program) {
	branch, ok := stmt.(*syntax.IfStmt)
	if ok {
		_CloseIf(branch, prog)

		return
	}

	suite := _SuiteOf(stmt)
	if suite != nil {
		_CloseBody(suite, prog)
	}
}

// _SuiteOf is the one suite a def, a for or a while holds, as a pointer so a
// caller can append to it, or nil for any other statement: an if holds two,
// which every caller treats apart, and the rest hold none.
//
// One place that knows which statements hold one suite, rather than a switch
// over the three in every walk of a body.
//
// Revisions:
//   - 2026-10-03 20:49: initial creation
func _SuiteOf(stmt syntax.Stmt) *[]syntax.Stmt {
	switch node := stmt.(type) {
	case *syntax.DefStmt:
		return &node.Body

	case *syntax.ForStmt:
		return &node.Body

	case *syntax.WhileStmt:
		return &node.Body

	default:
		return nil
	}
}

// _CloseIf closes an if. The false list of an elif is the next elif, not an
// else, so it is not closed.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _CloseIf(node *syntax.IfStmt, prog *_Program) {
	_CloseBody(&node.True, prog)

	if inner := _ElifOf(node, prog); inner != nil {
		_CloseIf(inner, prog)

		return
	}

	if len(node.False) == 0 {
		return
	}

	_CloseBody(&node.False, prog)
}

// _Ended reports that a suite is not given a closer: it is empty, it
// returns, or it already ends in pass.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Ended(body []syntax.Stmt) bool {
	if len(body) == 0 {
		return true
	}

	return _Returned(body[len(body)-1])
}

// _Returned reports that a suite already ends the way a closer would not.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
func _Returned(stmt syntax.Stmt) bool {
	switch node := stmt.(type) {
	case *syntax.ReturnStmt:
		return true

	case *syntax.BranchStmt:
		return node.Token == syntax.PASS

	default:
		return false
	}
}

// _ElifOf is the elif an else list holds, or nil when the list is an else.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _ElifOf(node *syntax.IfStmt, prog *_Program) *syntax.IfStmt {
	if len(node.False) != FIRST {
		return nil
	}

	inner, ok := node.False[0].(*syntax.IfStmt)
	if !ok {
		return nil
	}

	if _, marked := prog.elif[inner]; !marked {
		return nil
	}

	return inner
}

// _Parsed is body text as statements. The text is wrapped in a def so the
// parser will accept a suite, and that def is thrown away. A body that is
// only comments has no statement.
//
// Comments are retained, so the end-of-line ones can be cut from whatever is
// copied and each line comment placed where it was written.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-01 16:46: a body of only comments is no body
//   - 2026-10-02 00:11: works out where each line comment goes, and which
//     end-of-line comments to cut
//   - 2026-10-03 20:40: keeps the body as a text whose lines are found once
func _Parsed(prog *_Program, name, body string) ([]syntax.Stmt, error) {
	if _NotesOnly(body) {
		return nil, fmt.Errorf("%s: %w", name, ERR_NO_BODY)
	}

	wrapped := "def _():\n" + _Indent(body, INDENT)
	if !strings.HasSuffix(wrapped, "\n") {
		wrapped += "\n"
	}

	fileName := prog._File(name)

	file, err := dialect.OPTIONS.Parse(fileName, wrapped, syntax.RetainComments)
	if err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}

	def, ok := _Wrapped(file)
	if !ok {
		return nil, fmt.Errorf("script: %w", ERR_FORM)
	}

	text := _NewLines(wrapped)
	prog.sources[fileName] = text

	err = _Mark(text, def.Body, prog.elif)
	if err != nil {
		return nil, err
	}

	prog.cuts[fileName] = _Suffixes(file)

	_Place(prog, file, def)

	return def.Body, nil
}

// _NotesOnly reports that every line is blank or a comment, so the body
// has no statement.
//
// Revisions:
//   - 2026-10-01 16:46: initial creation
func _NotesOnly(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, COMMENT) {
			continue
		}

		return false
	}

	return true
}

// _Wrapped is the def the body was parsed under.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
func _Wrapped(file *syntax.File) (*syntax.DefStmt, bool) {
	if file == nil || len(file.Stmts) != FIRST {
		return nil, false
	}

	def, ok := file.Stmts[0].(*syntax.DefStmt)

	return def, ok
}

// _Mark remembers every elif in these statements. The word is read from the
// source, which is the only place the parser keeps it.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-03 20:40: reads the word from a text whose lines are found once
func _Mark(text *_Lines, stmts []syntax.Stmt, elif map[*syntax.IfStmt]struct{}) error {
	for _, stmt := range stmts {
		err := _MarkStmt(text, stmt, elif)
		if err != nil {
			return err
		}
	}

	return nil
}

// _MarkStmt remembers the elifs one statement contains.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-03 20:40: reads the word from a text whose lines are found once
//   - 2026-10-03 20:49: reaches a def's, a for's or a while's suite through _SuiteOf
func _MarkStmt(text *_Lines, stmt syntax.Stmt, elif map[*syntax.IfStmt]struct{}) error {
	branch, ok := stmt.(*syntax.IfStmt)
	if ok {
		return _MarkIf(text, branch, elif)
	}

	suite := _SuiteOf(stmt)
	if suite == nil {
		return nil
	}

	return _Mark(text, *suite, elif)
}

// _MarkIf remembers this if when its source word is elif, and walks both
// branches.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-03 20:40: reads the word from a text whose lines are found once
func _MarkIf(text *_Lines, node *syntax.IfStmt, elif map[*syntax.IfStmt]struct{}) error {
	err := _Mark(text, node.True, elif)
	if err != nil {
		return err
	}

	word, err := _Letters(text, node.If)
	if err != nil {
		return err
	}

	if word == ELIF {
		elif[node] = struct{}{}
	}

	return _Mark(text, node.False, elif)
}

// _Located is the source a node was parsed from, and the byte range of the
// node itself. A node built from the flow has no position and is not located.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-02 23:54: ends where _End says, past an index's or a slice's
//     closing bracket, which the interpreter's Span stops short of
//   - 2026-10-03 20:40: the source as a text whose lines are found once, so locating
//     a node no longer walks the source from its start
func _Located(prog *_Program, node syntax.Node) (*_Lines, int, int, bool) {
	if prog == nil || node == nil {
		return nil, 0, 0, false
	}

	start, _ := node.Span()
	end := _End(node)

	if !start.IsValid() || !end.IsValid() {
		return nil, 0, 0, false
	}

	text, ok := prog.sources[start.Filename()]
	if !ok {
		return nil, 0, 0, false
	}

	from := text._At(start.Line, start.Col)
	to := text._At(end.Line, end.Col)
	if from < 0 || to < from || to > len(text.src) {
		return nil, 0, 0, false
	}

	return text, from, to, true
}

// _End is where node's text ends: just past its last token.
//
// The interpreter's Span says so of every node but two. An IndexExpr and a
// SliceExpr end at their closing bracket rather than past it, and a node whose
// span ends with one of theirs inherits that end, so return got[1] would be
// sliced as return got[1 and lose the bracket. The end is therefore the
// furthest any node inside reaches, those two counted past their bracket.
//
// Revisions:
//   - 2026-10-02 23:54: initial creation
func _End(node syntax.Node) syntax.Position {
	_, end := node.Span()

	syntax.Walk(node, func(inner syntax.Node) bool {
		reach, closes := _Bracket(inner)
		further := closes &&
			(reach.Line > end.Line || reach.Line == end.Line && reach.Col > end.Col)

		if further {
			end = reach
		}

		return true
	})

	return end
}

// _Bracket is the position just past the closing bracket of an index or a
// slice, and whether node is one parsed from source. A bracket is one column
// wide, since columns count runes.
//
// Revisions:
//   - 2026-10-02 23:54: initial creation
func _Bracket(node syntax.Node) (syntax.Position, bool) {
	var bracket syntax.Position

	switch held := node.(type) {
	case *syntax.IndexExpr:
		bracket = held.Rbrack
	case *syntax.SliceExpr:
		bracket = held.Rbrack
	default:
		return syntax.Position{}, false
	}

	if !bracket.IsValid() {
		return syntax.Position{}, false
	}

	bracket.Col++

	return bracket, true
}

// _Span is the source text of a node, with nothing that follows it and with
// every end-of-line comment inside it cut. A line comment inside it, on a line
// of its own within a bracketed expression, stays.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-02 00:11: cuts the end-of-line comments inside the node
//   - 2026-10-03 20:40: cuts from a text whose lines are found once
func _Span(prog *_Program, node syntax.Node) (string, bool) {
	text, from, to, ok := _Located(prog, node)
	if !ok {
		return "", false
	}

	start, _ := node.Span()

	return _Cut(text, from, to, prog.cuts[start.Filename()]), true
}

// _Suffixes is every end-of-line comment in a parsed file, in the order they
// appear.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _Suffixes(file *syntax.File) []syntax.Comment {
	var found []syntax.Comment

	// Walk calls back once more with nil after each node's children.
	syntax.Walk(file, func(node syntax.Node) bool {
		if node == nil {
			return false
		}

		if notes := node.Comments(); notes != nil {
			found = append(found, notes.Suffix...)
		}

		return true
	})

	sort.Slice(found, func(i, j int) bool {
		return found[i].Start.Line < found[j].Start.Line
	})

	return found
}

// _Cut is the text from from to to without the end-of-line comments that fall
// inside it, and without the spaces before each one.
//
// cuts are in the order they appear, a line holding at most one, so the first
// inside the range is found by a binary search and the walk stops at the
// first past it. Visiting every comment of the file for every node cost the
// square of the comments, each of which also walked the source from its start.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
//   - 2026-10-03 20:40: reaches only the comments inside the range, in a text whose
//     lines are found once
func _Cut(text *_Lines, from, to int, cuts []syntax.Comment) string {
	var b strings.Builder

	at := from

	first := sort.Search(len(cuts), func(idx int) bool {
		return text._At(cuts[idx].Start.Line, cuts[idx].Start.Col) >= from
	})

	for _, note := range cuts[first:] {
		start := text._At(note.Start.Line, note.Start.Col)
		if start >= to {
			break
		}

		end := start + len(note.Text)
		lead := strings.TrimRight(text.src[at:start], " \t")

		b.WriteString(lead)

		at = end
	}

	b.WriteString(text.src[at:to])

	return b.String()
}

// _Letters is the letters at a position, which is how an elif is told from an
// if.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation, as _Word
//   - 2026-10-02 00:11: named apart from _Word, which reads a bare word's value
//   - 2026-10-03 20:40: reads from a text whose lines are found once
func _Letters(text *_Lines, pos syntax.Position) (string, error) {
	at := text._At(pos.Line, pos.Col)
	if at == MISSING {
		return "", fmt.Errorf("pass: %w", ERR_FORM)
	}

	var b strings.Builder

	for _, r := range text.src[at:] {
		if !unicode.IsLetter(r) {
			break
		}

		b.WriteRune(r)
	}

	return b.String(), nil
}
