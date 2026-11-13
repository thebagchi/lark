package graph

import (
	"bytes"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// _Unit is one resolved file of the script, for Reach: its tree, its text, the
// module each of its loads names, and the statements Reach keeps.
//
// Revisions:
//   - 2026-11-05 19:04: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-12 23:44: holds whether the text is odd, found once (C130, B289, Q402)
type _Unit struct {
	tree    *syntax.File
	text    []byte
	odd     bool
	loads   []*syntax.LoadStmt
	targets []*_Unit
	keep    []bool
	globals map[string]*resolve.Binding
}

// _Decl is one top-level statement that may be shaken out: a def, or an
// assignment of names.
//
// Revisions:
//   - 2026-11-05 19:04: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Decl struct {
	mod *_Unit
	idx int
}

// _Shake is the reach of a script: which statements a run can reach.
//
// decls maps the first identifier of each global a shakable statement defines
// to that statement. via maps the name a load binds to the global it stands
// for in the module it loads. work is the statements whose names are not yet
// read.
//
// Revisions:
//   - 2026-11-05 19:04: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Shake struct {
	decls map[*syntax.Ident]*_Decl
	via   map[*syntax.Ident]*resolve.Binding
	work  []*_Decl
}

// Reach sets Keep in every module of order, the entry among them, to the
// statements a run can reach. A compile (merge.Merge) and the derivation of
// the flow (Of) both call it, so the program and the flow list the same
// definitions and arguments (plan Q15, Q157, Q163, Q164).
//
// The rule:
//   - the entry's main is reached;
//   - every top-level statement that is not a load, a def or an assignment of
//     names is reached: a bare call such as print(...) or f(), an if, a for,
//     an augmented assignment, an assignment to an element or an attribute;
//   - an assignment of names (x = value, a, b = value) is reached only when a
//     name it binds is used by a reached statement, whatever the value is, a
//     call or an arg(...) declaration included;
//   - a def is reached only when a reached statement names it;
//   - a name is found by the resolver's Binding, never by a spelling, through
//     a load into the module that defines it.
//
// A function handed as a value, such as spawn(f) or retry(3, f), is a name in
// a reached statement, so it is reached. No builtin finds a function of the
// script by a string; the one name the run reads as a string is main.
//
// An entry with no main shakes nothing: nothing is reached, and a library
// would be emptied. A statement that shares a line with another is shaken as any
// other is, alone (Q400). Every statement of a module whose text is odd (see
// Odd) is reached: a text editor would cut in the wrong place.
//
// The arguments are go.starlark.net's own types. entry and order are resolved
// files, dependencies first, the entry among them: their identifiers carry
// their Binding. targets maps each load statement of those files to the file it
// loads, the same file for the same path. texts maps each file to its text, for
// Odd. Files the entry never loads do not belong in order.
//
// Returns, for each file of order, which of its top-level statements are
// reached: keep[i] is true when statement i stays. Returns nil when the entry
// has no main, and nothing is shaken.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from amalgam's _Shaken (C44b, Q164)
//   - 2026-11-05 18:00: takes *syntax.File and maps, and returns the answer, so
//     no new exported type appears in graph (C44b, Q182)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-12 22:12: a statement that shares a line is shaken alone (C130, B285, Q400)
//   - 2026-11-13 17:40: names merge.Merge, formerly amalgam.Merge (C134, Q446)
func Reach(
	entry *syntax.File,
	order []*syntax.File,
	targets map[*syntax.LoadStmt]*syntax.File,
	texts map[*syntax.File][]byte,
) map[*syntax.File][]bool {
	var (
		units = _Units(
			order,
			targets,
			texts,
		)
		main, ok = units[entry]._Globals()[common.ENTRY]
	)
	if !ok {
		return nil
	}

	shake := &_Shake{
		decls: map[*syntax.Ident]*_Decl{},
		via:   map[*syntax.Ident]*resolve.Binding{},
	}

	for _, tree := range order {
		shake._Via(units[tree])
	}

	for _, tree := range order {
		for idx, stmt := range tree.Stmts {
			shake._Classify(
				&_Decl{
					mod: units[tree],
					idx: idx,
				},
				stmt,
			)
		}
	}

	shake._Reach(main.First)

	for len(shake.work) > 0 {
		var (
			last = len(shake.work) - 1
			decl = shake.work[last]
		)

		shake.work = shake.work[:last]

		shake._Read(decl.mod.tree.Stmts[decl.idx])
	}

	kept := make(
		map[*syntax.File][]bool,
		len(order),
	)

	for tree, unit := range units {
		kept[tree] = unit.keep
	}

	return kept
}

// _Units is a unit for each file of order, linked to the units its loads name.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from Reach (C44b, Q182)
//   - 2026-11-12 23:44: finds once whether each text is odd (C130, B289, Q402)
func _Units(
	order []*syntax.File,
	targets map[*syntax.LoadStmt]*syntax.File,
	texts map[*syntax.File][]byte,
) map[*syntax.File]*_Unit {
	units := make(
		map[*syntax.File]*_Unit,
		len(order),
	)

	for _, tree := range order {
		units[tree] = &_Unit{
			tree: tree,
			text: texts[tree],
			odd:  Odd(texts[tree]),
			keep: make(
				[]bool,
				len(tree.Stmts),
			),
		}
	}

	for _, unit := range units {
		unit._Linked(
			targets,
			units,
		)
	}

	return units
}

// Odd reports whether the scanner of the parser reads text in a way that line
// splitting at line feeds does not. A NUL ends the file for the scanner, so
// what follows is no program. Such a text is not cut or shaken: the edits would
// stand in the wrong place.
//
// A carriage return that no line feed follows is a line break to the scanner
// too, but a source is read through script.Normalised before it gets here, which
// writes it as a line feed, so the text is not odd for it (Q405).
//
// Lines splits as the scanner does, so a line table is right for such text. Odd
// still guards the edits of package merge, which keep the line count of what they
// take out by writing one line feed for each line feed they remove.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from amalgam's _Odd (C44b)
//   - 2026-11-10 11:58: says why it stays now that Lines splits as the scanner does (C118, B260)
//   - 2026-11-12 23:41: a lone carriage return is not odd, as a source is normalised (C130, Q405)
//   - 2026-11-13 17:40: names package merge, formerly amalgam (C134, Q446)
func Odd(text []byte) bool {
	return bytes.IndexByte(
		text,
		0,
	) >= 0
}

// _Globals is the globals of the module by name, found the first time.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, Q164)
func (m *_Unit) _Globals() map[string]*resolve.Binding {
	if m.globals == nil {
		m.globals = map[string]*resolve.Binding{}

		resolved, ok := m.tree.Module.(*resolve.Module)
		if ok {
			for _, bind := range resolved.Globals {
				m.globals[bind.First.Name] = bind
			}
		}
	}

	return m.globals
}

// _Via records what each name a load of mod binds stands for.
//
// A name the target does not define is left out; the caller refuses it.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from amalgam (C44b, Q164)
func (s *_Shake) _Via(mod *_Unit) {
	for idx, load := range mod.loads {
		target := mod.targets[idx]

		for pos, from := range load.From {
			bind, ok := target._Globals()[from.Name]
			if !ok {
				continue
			}

			to, _ := load.To[pos].Binding.(*resolve.Binding)
			if to != nil {
				s.via[to.First] = bind
			}
		}
	}
}

// _Classify puts a statement in the roots, which stay, or in the decls, which
// stay only when something reaches them.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from amalgam (C44b, Q164)
//   - 2026-11-12 23:44: a statement that shares a line is no root; reads whether the text is odd
//     from the unit (C130, B285, B289, Q400, Q402)
func (s *_Shake) _Classify(
	decl *_Decl,
	stmt syntax.Stmt,
) {
	if decl.mod.odd {
		decl.mod.keep[decl.idx] = true
		s.work = append(
			s.work,
			decl,
		)

		return
	}

	switch held := stmt.(type) {
	case *syntax.LoadStmt:
		decl.mod.keep[decl.idx] = true

		return

	case *syntax.DefStmt:
		s.decls[held.Name] = decl

		return

	case *syntax.AssignStmt:
		names, ok := _Assigned(held)
		if ok {
			for _, name := range names {
				bind, _ := name.Binding.(*resolve.Binding)
				if bind != nil {
					s.decls[bind.First] = decl
				}
			}

			return
		}
	}

	decl.mod.keep[decl.idx] = true
	s.work = append(
		s.work,
		decl,
	)
}

// _Reach keeps the statement that defines the global first stands for.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
func (s *_Shake) _Reach(first *syntax.Ident) {
	for {
		bind, ok := s.via[first]
		if !ok {
			break
		}

		first = bind.First
	}

	decl, ok := s.decls[first]
	if !ok || decl.mod.keep[decl.idx] {
		return
	}

	decl.mod.keep[decl.idx] = true
	s.work = append(
		s.work,
		decl,
	)
}

// Read reaches every global stmt names.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-10 01:00: comment now names Read, were _Read (C120a, Q314)
func (s *_Shake) _Read(stmt syntax.Stmt) {
	syntax.Walk(
		stmt,
		func(node syntax.Node) bool {
			id, ok := node.(*syntax.Ident)
			if !ok {
				return true
			}

			bind, _ := id.Binding.(*resolve.Binding)
			if bind != nil {
				s._Reach(bind.First)
			}

			return true
		},
	)
}

// Assigned is the names an assignment binds, and whether it binds only names:
// `=` with names, or a tuple or a list of names, on the left. An augmented
// assignment and an assignment to an element or an attribute change a value
// that exists, so they bind nothing and are not shaken.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from _Constant (Q157)
//   - 2026-11-10 01:00: comment now names Assigned, were _Assigned (C120a, Q314)
func _Assigned(assign *syntax.AssignStmt) ([]*syntax.Ident, bool) {
	if assign.Op != syntax.EQ {
		return nil, false
	}

	return _Targeted(
		assign.LHS,
		nil,
	)
}

// _Targeted is the names expr assigns, or false when it assigns to anything
// else, an element or an attribute.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
func _Targeted(
	expr syntax.Expr,
	names []*syntax.Ident,
) ([]*syntax.Ident, bool) {
	var list []syntax.Expr

	switch held := expr.(type) {
	case *syntax.Ident:
		return append(
			names,
			held,
		), true

	case *syntax.ParenExpr:
		return _Targeted(
			held.X,
			names,
		)

	case *syntax.TupleExpr:
		list = held.List

	case *syntax.ListExpr:
		list = held.List

	default:
		return nil, false
	}

	for _, item := range list {
		var ok bool

		names, ok = _Targeted(
			item,
			names,
		)
		if !ok {
			return nil, false
		}
	}

	return names, true
}

// _Linked records the load statements of the unit and the unit each one names.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, Q182)
func (m *_Unit) _Linked(
	targets map[*syntax.LoadStmt]*syntax.File,
	units map[*syntax.File]*_Unit,
) {
	for _, stmt := range m.tree.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}

		target := units[targets[load]]
		if target == nil {
			continue
		}

		m.loads = append(
			m.loads,
			load,
		)
		m.targets = append(
			m.targets,
			target,
		)
	}
}
