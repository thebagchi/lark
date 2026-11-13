package graph

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Program is the file as a syntax tree, plus what that tree does not store.
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
//
// Revisions:
//   - 2026-11-05 23:24: the sources map holds *Lines in place of *_Lines (C60, Q188)
//   - 2026-11-10 00:25: renamed from _Program, as a test in tests/ needs it (C120a, Q314)
type Program struct {
	functions []*syntax.DefStmt
	constants []syntax.Stmt
	arguments []syntax.Stmt
	init      []syntax.Stmt
	main      *syntax.DefStmt
	elif      map[*syntax.IfStmt]struct{}
	sources   map[string]*Lines
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
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func (p *Program) _File(name string) string {
	p.seq++

	return fmt.Sprintf(
		"%s-%d.star",
		name,
		p.seq,
	)
}

// _Close appends pass to every suite that does not return and does not
// already end in pass. An elif is the false list holding one if, and that
// list is not itself a suite.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Close(prog *Program) {
	for _, def := range prog.functions {
		_CloseDef(
			def,
			prog,
		)
	}

	for _, stmt := range prog.init {
		_CloseStmt(
			stmt,
			prog,
		)
	}

	_CloseDef(
		prog.main,
		prog,
	)
}

// _CloseDef closes a function and the suites inside it.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _CloseDef(
	def *syntax.DefStmt,
	prog *Program,
) {
	if def == nil {
		return
	}

	_CloseBody(
		&def.Body,
		prog,
	)
}

// _CloseBody closes each statement, then the list itself when it needs a
// closer.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _CloseBody(
	body *[]syntax.Stmt,
	prog *Program,
) {
	for _, stmt := range *body {
		_CloseStmt(
			stmt,
			prog,
		)
	}

	if _Ended(*body) {
		return
	}

	*body = append(
		*body,
		_Pass(),
	)
}

// _CloseStmt closes the suites one statement contains.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-03 20:49: reaches a def's, a for's or a while's suite through _SuiteOf
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _CloseStmt(
	stmt syntax.Stmt,
	prog *Program,
) {
	branch, ok := stmt.(*syntax.IfStmt)
	if ok {
		_CloseIf(
			branch,
			prog,
		)

		return
	}

	suite := _SuiteOf(stmt)
	if suite != nil {
		_CloseBody(
			suite,
			prog,
		)
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
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _CloseIf(
	node *syntax.IfStmt,
	prog *Program,
) {
	_CloseBody(
		&node.True,
		prog,
	)

	if inner := _ElifOf(
		node,
		prog,
	); inner != nil {
		_CloseIf(
			inner,
			prog,
		)

		return
	}

	if len(node.False) == 0 {
		return
	}

	_CloseBody(
		&node.False,
		prog,
	)
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
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _ElifOf(
	node *syntax.IfStmt,
	prog *Program,
) *syntax.IfStmt {
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
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
//   - 2026-11-12 18:20: reads through _Read, with the wrapped source (C128, B282)
func _Parsed(
	prog *Program,
	name,
	body string,
) ([]syntax.Stmt, error) {
	return _Read(
		prog,
		name,
		body,
		_Wrapping,
	)
}

// _Margined is body text as statements that are written at the margin of the
// file, an init statement. The text is parsed as it is, with no def around it:
// under a def every line would carry the def's indent, and a statement over
// several lines would gain it each time it is written (B279).
//
// Revisions:
//   - 2026-11-12 18:20: initial creation (C128, B279)
func _Margined(
	prog *Program,
	name,
	body string,
) ([]syntax.Stmt, error) {
	return _Read(
		prog,
		name,
		body,
		_Flat,
	)
}

// _Read is body text as statements, parsed by source. A body that is only
// comments has no statement.
//
// Revisions:
//   - 2026-11-12 18:20: initial creation, the body of _Parsed (C128, B279, B282)
func _Read(
	prog *Program,
	name,
	body string,
	source func(
		string,
		string,
	) (*_Parse, error),
) ([]syntax.Stmt, error) {
	if _NotesOnly(body) {
		return nil, fmt.Errorf(
			"%s: function has no body: %w",
			unpack.QuotePrefix(name),
			ErrForm,
		)
	}

	var (
		fileName    = prog._File(name)
		parsed, err = source(
			fileName,
			body,
		)
	)
	if err != nil {
		return nil, err
	}

	return _Noted(
		&_Script{
			prog:     prog,
			file:     parsed.file,
			def:      parsed.def,
			wrapped:  parsed.text,
			fileName: fileName,
		},
	)
}

// _Parse is a body that has been parsed: the file, the text that was parsed,
// and the def whose body holds the statements.
//
// Revisions:
//   - 2026-11-12 18:21: initial creation (C128, B279, B282)
type _Parse struct {
	file *syntax.File
	text string
	def  *syntax.DefStmt
}

// _Wrapping parses body as the body of one function, and finds that function.
//
// Revisions:
//   - 2026-11-12 18:21: initial creation, from _Parsed (C128, B282)
func _Wrapping(
	fileName string,
	body string,
) (*_Parse, error) {
	file, wrapped, err := _WrappedSource(
		fileName,
		body,
	)
	if err != nil {
		return nil, err
	}

	def, ok := _Wrapped(file)
	if !ok {
		return nil, fmt.Errorf(
			"script: %w",
			ErrForm,
		)
	}

	return &_Parse{
		file: file,
		text: wrapped,
		def:  def,
	}, nil
}

// _Flat parses body as it is, as the statements of a file. The def that holds
// them is made, not parsed, so that the statements are read as any other.
//
// Revisions:
//   - 2026-11-12 18:21: initial creation (C128, B279)
func _Flat(
	fileName string,
	body string,
) (*_Parse, error) {
	text := body
	if !strings.HasSuffix(
		text,
		"\n",
	) {
		text += "\n"
	}

	file, err := common.OPTIONS.Parse(
		fileName,
		text,
		syntax.RetainComments,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"script: %w",
			err,
		)
	}

	return &_Parse{
		file: file,
		text: text,
		def: &syntax.DefStmt{
			Body: file.Stmts,
		},
	}, nil
}

// _Script is one body that has been parsed and is about to be recorded.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
type _Script struct {
	prog     *Program
	file     *syntax.File
	def      *syntax.DefStmt
	wrapped  string
	fileName string
}

// _WrappedSource parses body as the body of one function.
//
// Every line of body is indented under the def, except the lines inside a
// string. Which lines those are is known only from a parse, so body is parsed
// once with every line indented, and again, if that found a string over several
// lines, with those lines as they were written (B282).
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-12 18:47: leaves the lines of a string as they were (C128, B282)
func _WrappedSource(
	fileName string,
	body string,
) (*syntax.File, string, error) {
	file, wrapped, err := _Under(
		fileName,
		body,
		nil,
	)
	if err != nil {
		return nil, "", err
	}

	strung := _Strung(file)
	if len(strung) == 0 {
		return file, wrapped, nil
	}

	return _Under(
		fileName,
		body,
		strung,
	)
}

// _Under parses body under a def, with the lines in keep not indented.
//
// Revisions:
//   - 2026-11-12 18:21: initial creation, the body of _WrappedSource (C128, B282)
func _Under(
	fileName string,
	body string,
	keep map[int32]struct{},
) (*syntax.File, string, error) {
	wrapped := "def _():\n" + _Indent(
		body,
		INDENT,
		keep,
	)
	if !strings.HasSuffix(
		wrapped,
		"\n",
	) {
		wrapped += "\n"
	}

	file, err := common.OPTIONS.Parse(
		fileName,
		wrapped,
		syntax.RetainComments,
	)
	if err != nil {
		return nil, "", fmt.Errorf(
			"script: %w",
			err,
		)
	}

	return file, wrapped, nil
}

// _Noted records the source, the cuts, and where the function sits.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-05 23:24: makes the lines with NewLines in place of _NewLines (C60, Q188)
//   - 2026-11-12 19:05: finds the end-of-line comments with Trailing (C130, B286)
func _Noted(script *_Script) ([]syntax.Stmt, error) {
	var (
		fileName = script.fileName
		text     = NewLines(script.wrapped)
	)

	script.prog.sources[fileName] = text

	err := _Mark(
		text,
		script.def.Body,
		script.prog.elif,
	)
	if err != nil {
		return nil, err
	}

	cuts, err := Trailing(
		fileName,
		script.wrapped,
		script.file,
	)
	if err != nil {
		return nil, err
	}

	script.prog.cuts[fileName] = cuts

	_Place(
		script.prog,
		script.file,
		script.def,
	)

	return script.def.Body, nil
}

// _NotesOnly reports that every line is blank or a comment, so the body
// has no statement. A line ends where _LineEnd ends one, as the scanner of the
// parser does, so a statement after a lone carriage return is seen (B271).
//
// Revisions:
//   - 2026-10-01 16:46: initial creation
//   - 2026-11-11 06:40: ends a line with _LineEnd, so a lone carriage return ends one (C122, B271)
func _NotesOnly(body string) bool {
	for at := 0; at <= len(body); {
		var (
			end = _LineEnd(
				body,
				at,
			)
			line = strings.TrimSpace(body[at:end])
		)

		if line != "" && !strings.HasPrefix(
			line,
			COMMENT,
		) {
			return false
		}

		at = end + 1
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
//   - 2026-11-05 23:24: takes a *Lines in place of a *_Lines (C60, Q188)
func _Mark(
	text *Lines,
	stmts []syntax.Stmt,
	elif map[*syntax.IfStmt]struct{},
) error {
	for _, stmt := range stmts {
		err := _MarkStmt(
			text,
			stmt,
			elif,
		)
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
//   - 2026-11-05 23:24: takes a *Lines in place of a *_Lines (C60, Q188)
//   - 2026-11-10 01:00: now uses MarkIf, were _MarkIf (C120a, Q314)
func _MarkStmt(
	text *Lines,
	stmt syntax.Stmt,
	elif map[*syntax.IfStmt]struct{},
) error {
	branch, ok := stmt.(*syntax.IfStmt)
	if ok {
		return MarkIf(
			text,
			branch,
			elif,
		)
	}

	suite := _SuiteOf(stmt)
	if suite == nil {
		return nil
	}

	return _Mark(
		text,
		*suite,
		elif,
	)
}

// MarkIf remembers this if when its source word is elif, and walks both
// branches.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-10-03 20:40: reads the word from a text whose lines are found once
//   - 2026-11-05 23:24: takes a *Lines in place of a *_Lines (C60, Q188)
//   - 2026-11-06 22:02: a nil if marks nothing (C100c, B181)
//   - 2026-11-10 00:25: renamed from _MarkIf, as a test in tests/ needs it (C120a, Q314)
func MarkIf(
	text *Lines,
	node *syntax.IfStmt,
	elif map[*syntax.IfStmt]struct{},
) error {
	if node == nil {
		return nil
	}

	err := _Mark(
		text,
		node.True,
		elif,
	)
	if err != nil {
		return err
	}

	word, err := _Letters(
		text,
		node.If,
	)
	if err != nil {
		return err
	}

	if word == ELIF {
		elif[node] = struct{}{}
	}

	return _Mark(
		text,
		node.False,
		elif,
	)
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
//   - 2026-11-05 23:24: returns a *Lines, and finds places with Lines.At in place of _At (C60,
//     Q188)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Located(
	prog *Program,
	node syntax.Node,
) (*Lines, int, int, bool) {
	if prog == nil || node == nil {
		return nil, 0, 0, false
	}

	var (
		start, _ = node.Span()
		end      = _End(node)
	)

	if !start.IsValid() || !end.IsValid() {
		return nil, 0, 0, false
	}

	text, ok := prog.sources[start.Filename()]
	if !ok {
		return nil, 0, 0, false
	}

	var (
		from = text.At(
			start.Line,
			start.Col,
		)
		to = text.At(
			end.Line,
			end.Col,
		)
	)
	if from < 0 || to < from || to > len(text.src) {
		return nil, 0, 0, false
	}

	return text, from, to, true
}

// _End is where node's text ends: just past its last token.
//
// The interpreter's Span says so of every node but three. An IndexExpr and a
// SliceExpr end at their closing bracket rather than past it, and so does the
// empty tuple () or ( ), the one TupleExpr with parentheses of its own. A node
// whose span ends with one of those inherits that end, so return got[1] would
// be sliced as return got[1 and lose the bracket, and x = () as x = (. The end
// is therefore the furthest any node inside reaches, those three counted past
// their bracket.
//
// Revisions:
//   - 2026-10-02 23:54: initial creation
//   - 2026-11-10 11:53: counts the empty tuple past its parenthesis (C118, B258)
func _End(node syntax.Node) syntax.Position {
	_, end := node.Span()

	syntax.Walk(
		node,
		func(inner syntax.Node) bool {
			var reach, closes = _Bracket(inner)

			further := closes &&
				(reach.Line > end.Line ||
					reach.Line == end.Line && reach.Col > end.Col)

			if further {
				end = reach
			}

			return true
		},
	)

	return end
}

// _Bracket is the position just past the closing bracket of an index, a slice
// or an empty tuple, and whether node is one parsed from source. A bracket is
// one column wide, since columns count runes.
//
// A tuple has an Lparen only when it is empty: the parser wraps a tuple of
// elements in a ParenExpr, which ends past its parenthesis.
//
// Revisions:
//   - 2026-10-02 23:54: initial creation
//   - 2026-11-10 11:53: reads the closing parenthesis of an empty tuple (C118, B258)
func _Bracket(node syntax.Node) (syntax.Position, bool) {
	var bracket syntax.Position

	switch held := node.(type) {
	case *syntax.IndexExpr:
		bracket = held.Rbrack
	case *syntax.SliceExpr:
		bracket = held.Rbrack
	case *syntax.TupleExpr:
		if !held.Lparen.IsValid() {
			return syntax.Position{}, false
		}

		bracket = held.Rparen
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
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Span(
	prog *Program,
	node syntax.Node,
) (string, bool) {
	text, from, to, ok := _Located(
		prog,
		node,
	)
	if !ok {
		return "", false
	}

	start, _ := node.Span()

	return _Cut(
		text,
		from,
		to,
		prog.cuts[start.Filename()],
	), true
}

// _Suffixes is every end-of-line comment in a parsed file, in the order they
// appear.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _Suffixes(file *syntax.File) []syntax.Comment {
	var found []syntax.Comment

	// Walk calls back once more with nil after each node's children.
	syntax.Walk(
		file,
		func(node syntax.Node) bool {
			if node == nil {
				return false
			}

			if notes := node.Comments(); notes != nil {
				found = append(
					found,
					notes.Suffix...,
				)
			}

			return true
		},
	)

	sort.Slice(
		found,
		func(
			i,
			j int,
		) bool {
			return found[i].Start.Line < found[j].Start.Line
		},
	)

	return found
}

// Trailing is every end-of-line comment in a text, in the order they appear.
// file is the text parsed with its comments retained, and fileName the name the
// parser stored on its positions.
//
// The parser hands a suffix comment to the last node that ends before it, and
// each node takes one. A comment right after an opening bracket, in a statement
// that has few nodes before the next comment, is handed to none and is lost
// (B286). So the comments found are blanked in a copy of the text, which keeps
// every line and column, and the copy is read again until it holds no more. The
// text then derives the same cuts the second time it is read as it did the
// first. A comment found is a hash in the text and blanking removes it, so the
// rounds are no more than the hashes: the loop is bound by their count, and a
// round that finds a comment when no hash is left ends it with an error, in place
// of reading for ever.
//
// Returns ErrForm for a comment that is no part of text, and a wrapped error
// when the text with the comments blanked does not parse.
//
// Revisions:
//   - 2026-11-12 22:13: initial creation, from _Suffixes (C130, B286)
func Trailing(
	fileName string,
	text string,
	file *syntax.File,
) ([]syntax.Comment, error) {
	var found []syntax.Comment

	for budget := strings.Count(
		text,
		COMMENT,
	); ; budget-- {
		round := _Suffixes(file)
		if len(round) == 0 {
			break
		}

		if budget == 0 {
			return nil, fmt.Errorf(
				"comment: %w",
				ErrForm,
			)
		}

		found = append(
			found,
			round...,
		)

		blank, err := Blanked(
			text,
			round,
		)
		if err != nil {
			return nil, err
		}

		text = blank

		file, err = common.OPTIONS.Parse(
			fileName,
			text,
			syntax.RetainComments,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"script: %w",
				err,
			)
		}
	}

	sort.Slice(
		found,
		func(
			i,
			j int,
		) bool {
			return found[i].Start.Line < found[j].Start.Line
		},
	)

	return found, nil
}

// Blanked is text with each comment of notes written over with spaces, so every
// line and column stays. Returns ErrForm for a comment that is no part of text.
//
// Revisions:
//   - 2026-11-12 22:13: initial creation, out of _Trailing (C130, B286)
func Blanked(
	text string,
	notes []syntax.Comment,
) (string, error) {
	var (
		lines = NewLines(text)
		blank = []byte(text)
	)

	for _, note := range notes {
		start := lines.At(
			note.Start.Line,
			note.Start.Col,
		)
		if start == MISSING || start+len(note.Text) > len(blank) {
			return "", fmt.Errorf(
				"comment: %w",
				ErrForm,
			)
		}

		for at := start; at < start+len(note.Text); at++ {
			blank[at] = ' '
		}
	}

	return string(blank), nil
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
//   - 2026-11-05 23:24: takes a *Lines, and cuts with Lines.At in place of _At (C60, Q188)
func _Cut(
	text *Lines,
	from,
	to int,
	cuts []syntax.Comment,
) string {
	var b strings.Builder

	var (
		at    = from
		first = sort.Search(
			len(cuts),
			func(idx int) bool {
				return text.At(
					cuts[idx].Start.Line,
					cuts[idx].Start.Col,
				) >= from
			},
		)
	)

	for _, note := range cuts[first:] {
		start := text.At(
			note.Start.Line,
			note.Start.Col,
		)
		if start >= to {
			break
		}

		var (
			end  = start + len(note.Text)
			lead = strings.TrimRight(
				text.src[at:start],
				" \t",
			)
		)

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
//   - 2026-11-05 23:24: takes a *Lines, and reads from Lines.At in place of _At (C60, Q188)
func _Letters(
	text *Lines,
	pos syntax.Position,
) (string, error) {
	at := text.At(
		pos.Line,
		pos.Col,
	)
	if at == MISSING {
		return "", fmt.Errorf(
			"pass: %w",
			ErrForm,
		)
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
