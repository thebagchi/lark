package graph

import (
	"go.starlark.net/syntax"
)

// A line comment of a body is put back where it was written. The parser hands
// each one to the next node in the file whatever its indentation, which is
// right for a comment above a statement and wrong for one written at the end of
// a block, or above an elif or an else: those reach the statement after the
// block, a level further out. So where each goes is worked out here, by its
// line and its column, once, before anything is printed.

// _Place works out where every line comment of a parsed body goes.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _Place(prog *_Program, file *syntax.File, def *syntax.DefStmt) {
	_PlaceSuite(prog, def.Body, nil)

	notes := file.Comments()
	if notes == nil {
		return
	}

	for _, note := range notes.After {
		_Tail(prog, def.Body, note)
	}
}

// _PlaceSuite places the comments above each statement of a suite, and those
// of every suite inside them.
//
// owner is the if whose else this suite is, or nil. A comment above the
// suite's first statement written before the else line was written above the
// else, or at the end of the if's own block.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _PlaceSuite(prog *_Program, suite []syntax.Stmt, owner *syntax.IfStmt) {
	for idx, stmt := range suite {
		for _, note := range _Before(stmt) {
			clause := idx == 0 && owner != nil && note.Start.Line < owner.ElsePos.Line
			if clause {
				_PlaceClause(prog, owner, note, func() {
					prog.elses[owner] = append(prog.elses[owner], note)
				})

				continue
			}

			_PlaceAbove(prog, suite, idx, note)
		}

		_PlaceInner(prog, stmt)
	}
}

// _PlaceInner places the comments of the suites one statement holds.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
//   - 2026-10-03 20:49: reaches a def's, a for's or a while's suite through _SuiteOf
func _PlaceInner(prog *_Program, stmt syntax.Stmt) {
	branch, ok := stmt.(*syntax.IfStmt)
	if ok {
		_PlaceIf(prog, branch)

		return
	}

	suite := _SuiteOf(stmt)
	if suite != nil {
		_PlaceSuite(prog, *suite, nil)
	}
}

// _PlaceIf places the comments of an if: its block, an elif's, and an else's.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _PlaceIf(prog *_Program, node *syntax.IfStmt) {
	_PlaceSuite(prog, node.True, nil)

	if inner := _ElifOf(node, prog); inner != nil {
		for _, note := range _Before(inner) {
			_PlaceClause(prog, node, note, func() {
				prog.notes[inner] = append(prog.notes[inner], note)
			})
		}

		_PlaceIf(prog, inner)

		return
	}

	if len(node.False) == 0 {
		return
	}

	_PlaceSuite(prog, node.False, node)
}

// _PlaceClause places a comment written between an if's block and its elif
// or else: at the end of the block when it is written at the block's
// indentation or further in, and otherwise above the clause, by above.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _PlaceClause(prog *_Program, node *syntax.IfStmt, note syntax.Comment, above func()) {
	if _Column(node.True[0]) <= note.Start.Col {
		_Tail(prog, node.True, note)

		return
	}

	above()
}

// _PlaceAbove places a comment the parser handed to the statement at idx.
//
// Written further in than that statement, after a statement with a block of
// its own, it was written at the end of that block: it goes at the end of the
// deepest block its column reaches. Otherwise it was written above the
// statement, and goes there.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _PlaceAbove(prog *_Program, suite []syntax.Stmt, idx int, note syntax.Comment) {
	stmt := suite[idx]

	deeper := idx > 0 && note.Start.Col > _Column(stmt)
	if deeper {
		inner := _Block(suite[idx-1], prog)
		if inner != nil && _Column(inner[0]) <= note.Start.Col {
			_Tail(prog, inner, note)

			return
		}
	}

	prog.notes[stmt] = append(prog.notes[stmt], note)
}

// _Tail puts a comment at the end of the deepest block, from suite inward along
// each last statement's own block, whose indentation its column reaches.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _Tail(prog *_Program, suite []syntax.Stmt, note syntax.Comment) {
	for {
		inner := _Block(suite[len(suite)-1], prog)
		if inner == nil || _Column(inner[0]) > note.Start.Col {
			break
		}

		suite = inner
	}

	prog.tails[suite[0]] = append(prog.tails[suite[0]], note)
}

// _Block is the block a statement ends with, or nil for a statement without
// one: an if's else, or its own block when it has none, a for's, a while's
// and a def's body. An elif's block is the elif's own.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _Block(stmt syntax.Stmt, prog *_Program) []syntax.Stmt {
	switch node := stmt.(type) {
	case *syntax.IfStmt:
		if inner := _ElifOf(node, prog); inner != nil {
			return _Block(inner, prog)
		}

		if len(node.False) > 0 {
			return node.False
		}

		return node.True

	case *syntax.ForStmt:
		return node.Body

	case *syntax.WhileStmt:
		return node.Body

	case *syntax.DefStmt:
		return node.Body
	}

	return nil
}

// _Column is the column a statement starts at.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _Column(stmt syntax.Stmt) int32 {
	start, _ := stmt.Span()

	return start.Col
}

// _Before is the line comments the parser handed to a node, written above it.
//
// Revisions:
//   - 2026-10-02 00:11: initial creation
func _Before(node syntax.Node) []syntax.Comment {
	notes := node.Comments()
	if notes == nil {
		return nil
	}

	return notes.Before
}
