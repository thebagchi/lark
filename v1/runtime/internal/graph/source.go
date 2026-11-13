package graph

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	// ErrShadow is returned when a file declares a name that another file
	// reads as a builtin.
	//
	// Flat, one name is one thing. The declaration would take the place of the
	// builtin in the file that reads it, and that file would call the wrong
	// function with no message.
	//
	// Revisions:
	//   - 2026-11-02 20:49: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	ErrShadow = errors.New("a declaration hides a builtin another file reads")
)

// Module reads one loaded file, and everything it loads, into the flow being
// built.
//
// Dependencies first, so a module is declared before whatever loads it. A
// module reached twice by different paths is one module, keyed by the path it
// resolved to - so a diamond inlines its functions once rather than colliding
// with itself. The walk that decides this is script's, shared with a compile.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-01 23:57: reaches modules through Modules, the name the loading
//     interface took when Source became the struct a caller passes
//   - 2026-10-03 00:10: refuses a module reached while it is still being read,
//     rather than leaving it out of the flow
//   - 2026-11-02 17:53: follows the load through script's Walk, which a compile
//     shares, and writes only the words of the refusal (C43)
//   - 2026-11-05 18:00: returns the name the load resolved to (C44b, Q164)
//   - 2026-11-05 23:24: the walk's refusal passes through, in the words of a
//     compile (C60, Q187)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Module, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Module(
	from string,
	target string,
) (string, error) {
	if r == nil {
		return "", _NoReading()
	}

	return r.walk.Reach(
		from,
		target,
		r.Loaded,
	)
}

// Loaded parses name and declares what the file holds.
//
// The walk has put name on its chain and marked it read before this runs.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-24 20:00: refuses a chain longer than MAX_DEPTH
//   - 2026-11-02 17:53: receives the text from the walk, which now owns the
//     chain, the set and the depth check (C43)
//   - 2026-11-05 23:40: quotes and bounds the name in a parse refusal (C63, B128)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Loaded, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Loaded(
	name string,
	src []byte,
) error {
	if r == nil {
		return _NoReading()
	}

	tree, err := common.OPTIONS.Parse(
		name,
		src,
		0,
	)
	if err != nil {
		return _ParseRefusal(
			name,
			err,
		)
	}

	return r.Read(
		tree,
		name,
		string(src),
	)
}

// _ParseRefusal is the error for a module that does not parse. The name is a
// script's text, so it is quoted and bounded once. A syntax error already holds
// the name, unquoted and whole, so its words are written again from its parts.
//
// Revisions:
//   - 2026-11-05 23:40: initial creation (C63, B128, Q4)
func _ParseRefusal(
	name string,
	err error,
) error {
	var held syntax.Error
	if errors.As(
		err,
		&held,
	) {
		return fmt.Errorf(
			"%s:%d:%d: %s",
			unpack.QuotePrefix(name),
			held.Pos.Line,
			held.Pos.Col,
			held.Msg,
		)
	}

	return fmt.Errorf(
		"%s: %w",
		unpack.QuotePrefix(name),
		err,
	)
}

// Read reads one parsed file after whatever it loads: resolved, with its loads
// followed, and kept for Admitted.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-03 20:40: finds the file's lines once, for every function it declares
//   - 2026-11-02 17:53: notes the builtins the file reads (C43, B043)
//   - 2026-11-04 19:26: hands the file's lines to the constants, which carry
//     the entry's statements (C44c, B054)
//   - 2026-11-05 18:00: only reads; _Admitted declares, after the shake (C44b,
//     Q164)
//   - 2026-11-05 23:24: makes the lines with NewLines in place of _NewLines (C60, Q188)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Read, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-11 00:33: keeps no copy of the source for Reach (C122, B263)
func (r *Reading) Read(
	tree *syntax.File,
	name string,
	src string,
) error {
	if r == nil {
		return _NoReading()
	}

	r._Builtins(
		tree,
		name,
	)

	file := &_File{
		name: name,
		tree: tree,
		text: NewLines(src),
	}

	for _, stmt := range tree.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}

		target, err := r.Loads(
			load,
			name,
		)
		if err != nil {
			return err
		}

		held := r.read[target]
		if held == nil {
			continue
		}

		r.loaded[load] = held.tree
	}

	r.read[name] = file
	r.files = append(
		r.files,
		file,
	)

	return nil
}

// Whole reads the entry, and every file it loads, then declares what a run
// reaches.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, Q164)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Whole, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Whole(
	tree *syntax.File,
	src string,
) error {
	if r == nil {
		return _NoReading()
	}

	err := r.Read(
		tree,
		r.entry,
		src,
	)
	if err != nil {
		return err
	}

	return r.Admitted()
}

// Admitted declares what each file read holds, in the order the files were
// read, and leaves out what no run reaches.
//
// The shake is the compile's own (graph.Reach), applied to the trees the
// reading resolved: a def or an assignment nothing reached is not declared,
// so the flow lists the arguments and functions the compiled script uses.
// The entry is the last file read.
//
// Reach is handed no texts. It keeps a module whole when its text is odd (see
// Odd), because the compile cuts the text and an edit would stand in the wrong
// place. A derivation cuts nothing, so it shakes such a module as any other:
// the flow then lists what a run reaches, and the script Emit writes from it
// derives the same flow again (B263).
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from the second half of _Read (C44b,
//     Q164)
//   - 2026-11-05 18:00: reads the answer of Reach as a map of the trees (Q182)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Admitted, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-11 00:32: shakes a module whose text is odd too, handing Reach no texts (C122, B263)
func (r *Reading) Admitted() error {
	if r == nil {
		return _NoReading()
	}

	order := make(
		[]*syntax.File,
		0,
		len(r.files),
	)

	for _, file := range r.files {
		order = append(
			order,
			file.tree,
		)
	}

	keeps := Reach(
		r.files[len(r.files)-1].tree,
		order,
		r.loaded,
		nil,
	)

	for _, file := range r.files {
		for _, def := range _Defs(
			file.tree,
			keeps[file.tree],
		) {
			err := r.Declare(
				def,
				file.name,
				file.text,
			)
			if err != nil {
				return err
			}
		}

		err := r.Constants(
			file.tree,
			file.name,
			file.text,
			keeps[file.tree],
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// Shadows refuses a name a file declares that another file reads as a builtin.
//
// Flat, one name is one thing: the declaration would take the place of the
// builtin in the file that reads it. Names go in sorted order, so one
// script gives one refusal on every run.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43, B043)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-10 00:25: renamed from _Shadows, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Shadows() error {
	if r == nil {
		return _NoReading()
	}

	for _, name := range slices.Sorted(maps.Keys(r.owner)) {
		reader, read := r.builtin[name]
		if !read {
			continue
		}

		return fmt.Errorf(
			"%s in %s is read as a builtin in %s: %w",
			unpack.ShortText(name),
			unpack.QuotePrefix(r.owner[name]),
			unpack.QuotePrefix(reader),
			ErrShadow,
		)
	}

	return nil
}

// Loads follows one load statement, refusing one that renames.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: names a module that is not a string with its own
//     sentinel
//   - 2026-11-05 18:00: returns the name of the module it followed (C44b, Q164)
//   - 2026-11-05 23:40: quotes and bounds the module name in a refusal (C63, B128)
//   - 2026-11-06 22:02: a nil load is refused with ErrModule (C100c, B180)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: renamed from _Loads, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func (r *Reading) Loads(
	load *syntax.LoadStmt,
	from string,
) (string, error) {
	if load == nil || load.Module == nil {
		return "", fmt.Errorf(
			"%s: a load names no module: %w",
			unpack.QuotePrefix(from),
			ErrUnresolved,
		)
	}

	for idx := range load.From {
		if load.From[idx].Name != load.To[idx].Name {
			return "", fmt.Errorf(
				"%s as %s: load that renames %w",
				unpack.ShortText(load.To[idx].Name),
				unpack.ShortText(load.From[idx].Name),
				ErrCarry,
			)
		}
	}

	held, ok := load.Module.Value.(string)
	if !ok {
		return "", fmt.Errorf(
			"%s: a load names no module: %w",
			unpack.QuotePrefix(from),
			ErrUnresolved,
		)
	}

	return r.Module(
		from,
		held,
	)
}

// Declare adds a function to the flat list, or says which other module
// already has that name, or refuses a signature the flow cannot carry.
//
// Returns ErrDuplicate naming both modules, and ErrCarry naming the def.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: refuses a default, a *args or a **kwargs here, where
//     a refusal is an error, rather than recording it and emitting the def
//     without them
//   - 2026-10-03 20:40: keeps the module as a text whose lines are found once
//   - 2026-11-05 23:24: takes a *Lines in place of a *_Lines (C60, Q188)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-08 12:59: a nil reading reads nothing (C108, B187)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: renamed from _Declare, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func (r *Reading) Declare(
	def *syntax.DefStmt,
	module string,
	text *Lines,
) error {
	if r == nil {
		return _NoReading()
	}

	name := def.Name.Name

	if _, plain := Params(def); !plain {
		return fmt.Errorf(
			"%s in %s: signature %w",
			unpack.ShortText(name),
			unpack.QuotePrefix(module),
			ErrCarry,
		)
	}

	owner, known := r.owner[name]
	if known && owner != module {
		return fmt.Errorf(
			"%s in %s and %s: %w",
			unpack.ShortText(name),
			unpack.QuotePrefix(owner),
			unpack.QuotePrefix(module),
			ErrDuplicate,
		)
	}

	if !known {
		r.order = append(
			r.order,
			name,
		)
	}

	r.defs[name] = def
	r.text[name] = text
	r.owner[name] = module

	return nil
}

// _Builtins notes each name the file reads as a builtin, with the first file
// that reads it.
//
// Resolving is how the file says which reads are builtins: a name the file
// binds, or loads, is not one. Every name nothing binds counts as predeclared,
// since a flow does not know the plugins a host will add, and an unbound name
// that a module declares is the same defect. The resolver's own errors are not
// this walk's to report: a compile reports them, with the position.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43, B043)
//   - 2026-11-10 01:00: now uses Reading, were _Reading (C120a, Q314)
func (r *Reading) _Builtins(
	tree *syntax.File,
	module string,
) {
	_ = resolve.File(
		tree,
		_Always,
		starlark.Universe.Has,
	)

	syntax.Walk(
		tree,
		func(node syntax.Node) bool {
			ident, ok := node.(*syntax.Ident)
			if !ok {
				return true
			}

			bound, ok := ident.Binding.(*resolve.Binding)
			if !ok {
				return true
			}

			if bound.Scope != resolve.Predeclared && bound.Scope != resolve.Universal {
				return true
			}

			_, known := r.builtin[ident.Name]
			if !known {
				r.builtin[ident.Name] = module
			}

			return true
		},
	)
}

// _Always reports every name as predeclared.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43, B043)
func _Always(string) bool {
	return true
}

// _Defs is every top-level function a file defines that a run reaches, in the
// order it defines them. keep is the shake's answer, nil when nothing is shaken.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-11-05 18:00: leaves out the functions no run reaches (C44b, Q164)
func _Defs(
	tree *syntax.File,
	keep []bool,
) []*syntax.DefStmt {
	var defs []*syntax.DefStmt

	for idx, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok && (keep == nil || keep[idx]) {
			defs = append(
				defs,
				def,
			)
		}
	}

	return defs
}
