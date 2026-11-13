package merge

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Edit replaces the bytes from start to end with with.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Edit struct {
	start int
	end   int
	with  string
	line  int32
	col   int32
	load  bool
	drop  bool
}

// _Merged is the merge of the modules read: their pieces of text, and the trees
// of their copies renamed and with the loads taken out.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from _Written
//   - 2026-11-02 22:58: makes Written and Compiled, reusing the resolved file
//     when it is the only one
//   - 2026-11-05 18:00: shakes first, and adds one module at a time through
//     _Appended (C44b)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func (b *_Build) _Merged(entry string) (*Merged, error) {
	merged := &Merged{
		Compiled: &syntax.File{
			Path:    entry,
			Options: common.OPTIONS,
		},
		entry: entry,
	}

	b._Shaken(b.mods[entry])

	var whole []syntax.Stmt

	for _, mod := range b.order {
		all, err := b._Appended(
			merged,
			mod,
		)
		if err != nil {
			return nil, err
		}

		if len(b.order) > 1 {
			whole = append(
				whole,
				all...,
			)
		}
	}

	return merged, b._Resolved(
		merged,
		whole,
	)
}

// _Appended adds mod to the merge: its file, its piece of text and the
// statements the shake kept. It returns every statement of mod's copy.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from the body of _Merged (C44b)
//   - 2026-11-05 18:00: finds the string ranges for the comments (Q157)
func (b *_Build) _Appended(
	merged *Merged,
	mod *_Module,
) ([]syntax.Stmt, error) {
	locals, err := b._Loaded(mod)
	if err != nil {
		return nil, err
	}

	mod.edits = _Edited(
		mod,
		locals,
	)

	sort.Slice(
		mod.edits,
		func(
			i,
			j int,
		) bool {
			return mod.edits[i].start < mod.edits[j].start
		},
	)

	merged.Files = append(
		merged.Files,
		mod.tree,
	)
	spans, strip := mod._Spans()

	merged.pieces = append(
		merged.pieces,
		&_Piece{
			name:  mod.name,
			text:  mod.text,
			edits: mod.edits,
			spans: spans,
			strip: strip,
		},
	)

	all, kept := _Renamings(mod)

	for idx, stmt := range mod.fresh.Stmts {
		_, load := stmt.(*syntax.LoadStmt)
		if !load {
			b.reach = append(
				b.reach,
				mod.keep == nil || mod.keep[idx],
			)
		}
	}

	merged.Compiled.Stmts = append(
		merged.Compiled.Stmts,
		kept...,
	)

	return all, nil
}

// _Resolved sets Written. The only file is already resolved, and nothing in it
// was renamed; a merge of many is copied, then resolved.
//
// Written holds every statement, those shaken out too: a refusal that reads
// the written tree, such as a bad spawn in a function nothing calls, is given
// as it was before the shake.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-05 18:00: copies the whole merge, not the compiled tree, which the
//     shake cut (C44b)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func (b *_Build) _Resolved(
	merged *Merged,
	whole []syntax.Stmt,
) error {
	if len(b.order) == 1 {
		merged.Written = b.order[0].tree
		merged.Kept = b._Kept(merged.Written)

		return nil
	}

	written, err := dialect.Copied(&syntax.File{
		Path:    merged.Compiled.Path,
		Stmts:   whole,
		Options: common.OPTIONS,
	})
	if err != nil {
		return err
	}

	err = resolve.File(
		written,
		b.env.Has,
		starlark.Universe.Has,
	)
	if err != nil {
		return err
	}

	merged.Written = written
	merged.Kept = b._Kept(written)

	return nil
}

// _Kept is written with only the statements the shake kept, the nodes
// shared with written. It holds the same statements as the compiled tree.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (Q157)
func (b *_Build) _Kept(written *syntax.File) *syntax.File {
	kept := *written
	kept.Stmts = make(
		[]syntax.Stmt,
		0,
		len(written.Stmts),
	)

	for idx, stmt := range written.Stmts {
		if idx < len(b.reach) && b.reach[idx] {
			kept.Stmts = append(
				kept.Stmts,
				stmt,
			)
		}
	}

	return &kept
}

// _Piece is the text of one file and the edits that turn it into its part of
// the merge. It holds no tree, so a merge that has been compiled keeps only
// what writing its text needs.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Piece struct {
	name  string
	text  []byte
	edits []*_Edit
	spans [][2]int
	strip bool
}

// _Spliced is the text of the piece in the pieces its edits cut it into: the
// text between two edits, and the text an edit puts in place of what it
// replaced. The comments are among the edits, each replaced by nothing.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from the proof of concept,
//     which wrote a comment in place of a load
//   - 2026-11-02 22:58: reads the edits _Merged sorted, and cannot fail
//   - 2026-11-02 22:58: a method of the piece, which holds the text and the
//     edits and no tree
//   - 2026-11-05 18:00: takes the comments out too, and may fail (C44b)
//   - 2026-11-05 18:00: cannot fail again (Q157, item 4)
func (p *_Piece) _Spliced() []string {
	var (
		edits = p._Stripped()
		parts []string
		at    = 0
	)

	for _, edit := range edits {
		parts = append(
			parts,
			string(p.text[at:edit.start]),
			edit.with,
		)
		at = edit.end
	}

	return append(
		parts,
		string(p.text[at:]),
	)
}

// _Renamings is the statements of mod's copy, without its loads, with every
// identifier the edits rename written under its final spelling; and those of
// them that the shake kept.
//
// The copy is matched to the edits by position: a copy keeps the line and the
// column of every node it was made from, and a statement of the copy to a
// statement of the parsed file by its place in the list.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-05 18:00: also returns the statements the shake kept (C44b)
func _Renamings(mod *_Module) ([]syntax.Stmt, []syntax.Stmt) {
	names := map[[2]int32]string{}

	for _, edit := range mod.edits {
		if !edit.load && !edit.drop {
			names[[2]int32{edit.line, edit.col}] = edit.with
		}
	}

	if len(names) == 0 && len(mod.loads) == 0 && mod.keep == nil {
		return mod.fresh.Stmts, mod.fresh.Stmts
	}

	_Respelled(
		mod.fresh,
		names,
	)

	var (
		all = make(
			[]syntax.Stmt,
			0,
			len(mod.fresh.Stmts),
		)
		kept = all
	)

	if mod.keep != nil {
		kept = make(
			[]syntax.Stmt,
			0,
			len(mod.fresh.Stmts),
		)
	}

	for idx, stmt := range mod.fresh.Stmts {
		_, load := stmt.(*syntax.LoadStmt)
		if load {
			continue
		}

		all = append(
			all,
			stmt,
		)

		if mod.keep != nil && mod.keep[idx] {
			kept = append(
				kept,
				stmt,
			)
		}
	}

	if mod.keep == nil {
		return all, all
	}

	return all, kept
}

// _Respelled writes the new spelling of each identifier of file that names
// holds, by the line and the column the identifier stands at.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from _Renamings (C44b)
func _Respelled(
	file *syntax.File,
	names map[[2]int32]string,
) {
	if len(names) == 0 {
		return
	}

	syntax.Walk(
		file,
		func(node syntax.Node) bool {
			id, ok := node.(*syntax.Ident)
			if !ok {
				return true
			}

			name, hit := names[[2]int32{id.NamePos.Line, id.NamePos.Col}]
			if hit {
				id.Name = name
			}

			return true
		},
	)
}

// _Edited is every edit mod needs: each load replaced by as many line breaks
// as it spanned, so every other line of the module keeps its number, and each
// identifier whose final spelling differs from the one written.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-05 18:00: also replaces each shaken statement by its line breaks
//     (C44b)
func _Edited(
	mod *_Module,
	locals map[*syntax.Ident]string,
) []*_Edit {
	var edits []*_Edit

	if len(mod.loads) == 0 && !mod._Renamed() && mod.keep == nil {
		return edits
	}

	walker := func(node syntax.Node) bool {
		switch held := node.(type) {
		case *syntax.LoadStmt:
			edits = append(
				edits,
				_Unloaded(
					mod,
					held,
				),
			)

			return false

		case *syntax.Ident:
			edit, ok := _Renamed(
				mod,
				held,
				locals,
			)
			if ok {
				edits = append(
					edits,
					edit,
				)
			}
		}

		return true
	}

	for idx, stmt := range mod.tree.Stmts {
		if mod._Shaken(idx) {
			edits = append(
				edits,
				_Shaken(
					mod,
					idx,
				),
			)

			continue
		}

		syntax.Walk(
			stmt,
			walker,
		)
	}

	return edits
}

// _Shaken is the edit that takes statement idx of mod out and keeps its lines,
// from its first byte to the end of its last line.
//
// A statement that another follows is taken out alone, on its line or over a
// line continuation: it becomes an empty tuple (see _Parened) before its
// semicolon, which stays, so the statements that stay keep their line and their
// column (Q400) and the merge parses (B291). Only a semicolon can join two
// statements, as a compound statement stands alone, and graph.Semicolon finds it,
// as the derivation does (B288).
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-05 23:26: finds the cut with _Module._At and _End in place of _Lines methods
//     (C60, Q188)
//   - 2026-11-12 23:38: cuts a statement that another follows on its line alone, as an empty tuple
//     before its semicolon, not the whole line (C130, B285, B291, Q400)
func _Shaken(
	mod *_Module,
	idx int,
) *_Edit {
	first, last := mod.tree.Stmts[idx].Span()

	var (
		start = mod._At(first)
		end   = mod._End(last.Line)
	)

	// Another statement follows when a semicolon joins it: only a semicolon
	// joins two statements, on a line or over a line continuation (B291). One at
	// the end of a line, or before a comment, joins nothing, and the line is cut
	// whole as before (B285).
	cut, joined := graph.Semicolon(
		mod.text,
		mod._At(last),
	)
	if joined && graph.Following(
		mod.text,
		cut,
	) {
		return &_Edit{
			start: start,
			end:   cut - 1,
			with:  _Parened(mod.text[start : cut-1]),
			drop:  true,
		}
	}

	// A semicolon that ends the logical line may stand on a later physical line,
	// after a line continuation: it goes with the statement, to the end of its
	// line, or the text would keep a semicolon with nothing before it (B292).
	if joined {
		end = max(
			end,
			cut+_Line(mod.text[cut:]),
		)
	}

	return &_Edit{
		start: start,
		end:   end,
		with: strings.Repeat(
			"\n",
			bytes.Count(
				mod.text[start:end],
				[]byte("\n"),
			),
		),
		drop: true,
	}
}

// _Line is the length of the first line of text, before its line feed, or of all
// of it when it holds none.
//
// Revisions:
//   - 2026-11-13 01:18: initial creation (C130, B292)
func _Line(text []byte) int {
	if at := bytes.IndexByte(
		text,
		'\n',
	); at >= 0 {
		return at
	}

	return len(text)
}

// _Parened is the text that stands in place of a shaken statement and the bytes
// up to its semicolon, which stays: an empty tuple, "(" and ")" at the first and
// last byte of the statement, with every rune between them written as a space,
// except a line break. It is a statement that does nothing, it spans the same
// lines and the same columns, so the statements after it keep their place, and
// the merge is a script that parses: spaces alone would leave the next statement
// indented. What stands after the statement, spaces and line continuations, is
// kept. A shaken statement is an assignment of names, at least three bytes.
//
// Revisions:
//   - 2026-11-12 23:37: initial creation (C130, B285, B291, Q400)
func _Parened(text []byte) string {
	stop := len(text)

	for stop > 0 && bytes.IndexByte(
		[]byte(" \t\r\n\\"),
		text[stop-1],
	) >= 0 {
		stop--
	}

	var b strings.Builder

	b.WriteByte('(')

	for _, char := range string(text[1 : stop-1]) {
		if char == '\n' || char == '\r' {
			b.WriteRune(char)

			continue
		}

		b.WriteByte(' ')
	}

	b.WriteByte(')')
	b.Write(text[stop:])

	return b.String()
}

// _Unloaded is the edit that takes a load out and keeps its lines.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-05 23:24: finds the cut with _Module._At in place of _Lines._At (C60, Q188)
func _Unloaded(
	mod *_Module,
	load *syntax.LoadStmt,
) *_Edit {
	last := load.Rparen
	last.Col++

	var (
		start = mod._At(load.Load)
		end   = mod._At(last)
	)

	return &_Edit{
		start: start,
		end:   end,
		with: strings.Repeat(
			"\n",
			bytes.Count(
				mod.text[start:end],
				[]byte("\n"),
			),
		),
	}
}

// _Renamed is the edit that writes the final spelling of id, when it differs
// from the one written.
//
// First is the one identifier every use of a variable shares: a use inside a
// def gets a Binding of its own, a Free one, but that copies First from the
// binding it closes over.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-05 23:24: finds the name with _Module._At in place of _Lines._At (C60, Q188)
func _Renamed(
	mod *_Module,
	id *syntax.Ident,
	locals map[*syntax.Ident]string,
) (*_Edit, bool) {
	bind, _ := id.Binding.(*resolve.Binding)
	if bind == nil {
		return nil, false
	}

	name, ok := mod.final[bind.First]
	if !ok {
		name, ok = locals[bind.First]
	}

	if !ok || name == id.Name {
		return nil, false
	}

	start := mod._At(id.NamePos)

	return &_Edit{
		start: start,
		end:   start + len(id.Name),
		with:  name,
		line:  id.NamePos.Line,
		col:   id.NamePos.Col,
	}, true
}

// _Loaded maps each name a module's loads bind to the final spelling of the
// global it stands for, and refuses a spelling a local of the module would
// capture.
//
// Returns ErrExport for a name the target does not define, and ErrCapture
// when the final spelling differs from the one the load bound and a function of
// the module has a local of that spelling.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from the proof of concept
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-10 17:15: uses ErrExport, formerly ErrNoExport (C121, Q348)
func (b *_Build) _Loaded(mod *_Module) (map[*syntax.Ident]string, error) {
	locals := map[*syntax.Ident]string{}

	for idx, load := range mod.loads {
		target := mod.targets[idx]

		for pos, from := range load.From {
			bind, ok := target.globals[from.Name]
			if !ok {
				return nil, fmt.Errorf(
					"%s in %s: %w",
					unpack.ShortText(from.Name),
					unpack.QuotePrefix(target.name),
					ErrExport,
				)
			}

			name := target.final[bind.First]
			if mod._Local()[name] && name != load.To[pos].Name {
				return nil, fmt.Errorf(
					"%s in %s: %w",
					unpack.ShortText(name),
					unpack.QuotePrefix(mod.name),
					ErrCapture,
				)
			}

			to, _ := load.To[pos].Binding.(*resolve.Binding)
			locals[to.First] = name
		}
	}

	return locals, nil
}

// _Shaken reports whether the shake left statement idx out.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
func (m *_Module) _Shaken(idx int) bool {
	return m.keep != nil && !m.keep[idx]
}

// _At is the byte offset of a position in the module's text, or the end of the
// text when the text has no such place.
//
// Revisions:
//   - 2026-11-05 23:24: initial creation, over graph.Lines in place of a copy
//     (C60, Q188)
func (m *_Module) _At(pos syntax.Position) int {
	at := m._Lines().At(
		pos.Line,
		pos.Col,
	)
	if at == graph.MISSING {
		return len(m.text)
	}

	return at
}

// _End is the byte offset where line ends, before its line break, or the end of
// the text when the text has no such line.
//
// Revisions:
//   - 2026-11-05 23:24: initial creation, over graph.Lines in place of a copy
//     (C60, Q188)
func (m *_Module) _End(line int32) int {
	end := m._Lines().End(line)
	if end == graph.MISSING {
		return len(m.text)
	}

	return end
}

// _Lines is the line starts of the module's text, found the first time an edit
// needs a place in it. A script with nothing to rename never needs them.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-05 23:24: a graph.Lines in place of a copy of its own (C60, Q188)
func (m *_Module) _Lines() *graph.Lines {
	if m.lines == nil {
		m.lines = graph.NewLines(string(m.text))
	}

	return m.lines
}

// _Local is every spelling a function, parameter or comprehension of the module
// binds below the top level, found the first time a rename needs it.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func (m *_Module) _Local() map[string]bool {
	if m.local == nil {
		m.local = _Locals(m.tree)
	}

	return m.local
}

// _Renamed is true when a global of the module has a final spelling other than
// the one written.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func (m *_Module) _Renamed() bool {
	for _, bind := range m.order {
		if m.final[bind.First] != bind.First.Name {
			return true
		}
	}

	return false
}
