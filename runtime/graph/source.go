package graph

import (
	"errors"
	"fmt"
	"os"
	"path"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/runtime/dialect"
)

var (
	// ErrCollision is returned when two modules declare one name. It names
	// both, because a message naming one leaves a reader looking for the
	// other.
	//
	// The limit is real and is the cost of the shape a user interface wants: a
	// Graph has functions and threads and no module, so a flat list cannot
	// hold two things called the same thing. Renaming is not the way out -
	// that would mean editing a body to call the renamed function, and
	// derivation never rewrites a body.
	ErrCollision = errors.New("two modules declare one name")

	// ErrAlias is returned for a load that renames what it binds.
	//
	// Flat, a name is one thing. load("strings.star", shout = "yell") means
	// every body in that file says shout while the function is yell, and
	// reconciling that means rewriting bodies.
	ErrAlias = errors.New("a load that renames cannot be inlined")

	// ErrModule is returned for a load whose module is not a string, which the
	// parser does not produce; it exists so the assertion has a sentinel
	// rather than borrowing one that means something else.
	ErrModule = errors.New("a load names no module")
)

// Source is where a module's text comes from.
//
// Declared here rather than imported so that generating a script does not
// depend on compiling one. Any loader a host already has satisfies it.
type Source interface {
	Resolve(from string, target string) (string, error)
	Load(name string) ([]byte, error)
}

// _Dir is the source used when a caller names none: a module is a file beside
// the one that loaded it, which is what a compiler does by default.
//
// Empty: it needs no state, because the file doing the loading is an argument.
type _Dir struct{}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (d *_Dir) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load reads a module's text.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (d *_Dir) Load(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// _Module reads one loaded file, and everything it loads, into the graph being
// built.
//
// Dependencies first, so a module is declared before whatever loads it. A
// module reached twice by different paths is one module, keyed by the path it
// resolved to - so a diamond inlines its functions once rather than colliding
// with itself.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Module(from string, target string) error {
	name, err := r.source.Resolve(from, target)
	if err != nil {
		return fmt.Errorf("%s: %w", target, err)
	}

	// The chain before the set. A module reached while it is still being read
	// is a cycle, and the entry is on that chain too - checking what has been
	// loaded first would answer "already have it" for a cycle that closes back
	// onto the file the derivation started from.
	for _, held := range r.loading {
		if held == name {
			r._Gave(name, "is loaded while it is still being read")

			return nil
		}
	}

	if r.loaded[name] {
		return nil
	}

	src, err := r.source.Load(name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	tree, err := dialect.OPTIONS.Parse(name, src, 0)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	r.loading = append(r.loading, name)
	defer func() { r.loading = r.loading[:len(r.loading)-1] }()

	r.loaded[name] = true

	return r._Read(tree, name, string(src))
}

// _Read declares everything one parsed file holds, after whatever it loads.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
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

	for _, def := range _Defs(tree) {
		err := r._Declare(def, name, src)
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
			return fmt.Errorf("%s as %s: %w", load.To[idx].Name, load.From[idx].Name, ErrAlias)
		}
	}

	held, ok := load.Module.Value.(string)
	if !ok {
		return fmt.Errorf("%s: %w", from, ErrModule)
	}

	return r._Module(from, held)
}

// _Declare adds a function to the flat list, or says which other module
// already has that name, or refuses a signature the graph cannot carry.
//
// Returns ErrCollision naming both modules, and ErrSignature naming the def.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: refuses a default, a *args or a **kwargs here, where
//     a refusal is an error, rather than recording it and emitting the def
//     without them
func (r *_Reading) _Declare(def *syntax.DefStmt, module string, src string) error {
	name := def.Name.Name

	if _, plain := _Params(def); !plain {
		return fmt.Errorf("%s in %s: %w", name, module, ErrSignature)
	}

	owner, known := r.owner[name]
	if known && owner != module {
		return fmt.Errorf("%s in %s and %s: %w", name, owner, module, ErrCollision)
	}

	if !known {
		r.order = append(r.order, name)
	}

	r.defs[name] = def
	r.text[name] = src
	r.owner[name] = module

	return nil
}
