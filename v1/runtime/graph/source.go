package graph

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

var (
	// ERR_COLLISION is returned when two modules declare one name. It names
	// both, because a message naming one leaves a reader looking for the
	// other.
	//
	// The limit is real and is the cost of the shape a user interface wants: a
	// Flow has functions and no module, so a flat list cannot hold two things
	// called the same thing. Renaming is not the way out - that would mean
	// editing a body to call the renamed function, and derivation never
	// rewrites a body.
	ERR_COLLISION = errors.New("two modules declare one name")

	// ERR_ALIAS is returned for a load that renames what it binds.
	//
	// Flat, a name is one thing. load("strings.star", shout = "yell") means
	// every body in that file says shout while the function is yell, and
	// reconciling that means rewriting bodies.
	ERR_ALIAS = errors.New("a load that renames cannot be inlined")

	// ERR_MODULE is returned for a load whose module is not a string, which the
	// parser does not produce; it exists so the assertion has a sentinel
	// rather than borrowing one that means something else.
	ERR_MODULE = errors.New("a load names no module")
)

// _Module reads one loaded file, and everything it loads, into the flow being
// built.
//
// Dependencies first, so a module is declared before whatever loads it. A
// module reached twice by different paths is one module, keyed by the path it
// resolved to - so a diamond inlines its functions once rather than colliding
// with itself.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-01 23:57: reaches modules through Modules, the name the loading
//     interface took when Source became the struct a caller passes
//   - 2026-10-03 00:10: refuses a module reached while it is still being read,
//     rather than leaving it out of the flow
func (r *_Reading) _Module(from string, target string) error {
	name, err := r.modules.Resolve(from, target)
	if err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}

	// The chain before the set. A module reached while it is still being read
	// is a cycle, and the entry is on that chain too - checking what has been
	// loaded first would answer "already have it" for a cycle that closes back
	// onto the file the derivation started from.
	for _, held := range r.loading {
		if held == name {
			return fmt.Errorf(
				"%s is loaded while it is still being read: %w",
				name,
				ERR_CYCLE,
			)
		}
	}

	if r.loaded[name] {
		return nil
	}

	src, err := r.modules.Load(name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	tree, err := dialect.OPTIONS.Parse(name, src, 0)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	r.loading = append(r.loading, name)
	defer func() {
		r.loading = r.loading[:len(r.loading)-1]
	}()

	r.loaded[name] = true

	return r._Read(tree, name, string(src))
}

// _Read declares everything one parsed file holds, after whatever it loads.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-03 20:40: finds the file's lines once, for every function it declares
func (r *_Reading) _Read(tree *syntax.File, name string, src string) error {
	for _, stmt := range tree.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}

		err := r._Loads(load, name)
		if err != nil {
			return err
		}
	}

	text := _NewLines(src)

	for _, def := range _Defs(tree) {
		err := r._Declare(def, name, text)
		if err != nil {
			return err
		}
	}

	return r._Constants(tree, name)
}

// _Loads follows one load statement, refusing one that renames.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: names a module that is not a string with its own
//     sentinel
func (r *_Reading) _Loads(load *syntax.LoadStmt, from string) error {
	for idx := range load.From {
		if load.From[idx].Name != load.To[idx].Name {
			return fmt.Errorf("%s as %s: %w",
				load.To[idx].Name, load.From[idx].Name, ERR_ALIAS)
		}
	}

	held, ok := load.Module.Value.(string)
	if !ok {
		return fmt.Errorf("%s: %w", from, ERR_MODULE)
	}

	return r._Module(from, held)
}

// _Declare adds a function to the flat list, or says which other module
// already has that name, or refuses a signature the flow cannot carry.
//
// Returns ERR_COLLISION naming both modules, and ERR_SIGNATURE naming the def.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: refuses a default, a *args or a **kwargs here, where
//     a refusal is an error, rather than recording it and emitting the def
//     without them
//   - 2026-10-03 20:40: keeps the module as a text whose lines are found once
func (r *_Reading) _Declare(def *syntax.DefStmt, module string, text *_Lines) error {
	name := def.Name.Name

	if _, plain := _Params(def); !plain {
		return fmt.Errorf("%s in %s: %w", name, module, ERR_SIGNATURE)
	}

	owner, known := r.owner[name]
	if known && owner != module {
		return fmt.Errorf("%s in %s and %s: %w", name, owner, module, ERR_COLLISION)
	}

	if !known {
		r.order = append(r.order, name)
	}

	r.defs[name] = def
	r.text[name] = text
	r.owner[name] = module

	return nil
}

// _Defs is every top-level function a file defines, in the order it defines
// them.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Defs(tree *syntax.File) []*syntax.DefStmt {
	var defs []*syntax.DefStmt

	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok {
			defs = append(defs, def)
		}
	}

	return defs
}
