// Package script is a script as the runtime reads it: the main script's name
// and text, and what reaches the modules it loads.
//
// Its own package because two sides read one script: a compile, which builds an
// artifact from it, and a derivation, which builds the flow it describes. Each
// declaring its own shape would be two declarations of one thing, and a host
// holding a script would convert between them.
package script

import (
	"os"
	"path"
)

// Loader fetches the source of a module a script asked to load, and says what
// that module is actually called.
//
// A host serving scripts from a directory, an archive, a database or a test
// map implements this without having to be, or to own, a file system.
//
// Resolving is separate from fetching, and the split keeps two properties: a
// module reached by two routes is fetched once, and a cycle is refused without
// fetching anything. Both need to know what a spelling means before deciding
// whether to ask for it.
//
// Resolve is given the module doing the loading and the spelling it used, and
// returns the identity the module is filed under. Two spellings that reach one
// module must resolve to one name, or it is read twice; one spelling that
// reaches two modules must resolve to two, or the wrong one is reused. It is
// expected to be cheap - a path join, a key normalisation - because it runs for
// every load whether or not a fetch follows.
type Loader interface {
	Resolve(from string, target string) (string, error)
	Load(name string) ([]byte, error)
}

// Source is a script: the main script's name and text, and the Loader that
// finds the modules it loads.
//
// A struct rather than three parameters, so the main script and the modules it
// reaches arrive as one thing. Exported fields, because a caller in another
// package builds one and nothing here keeps them in step. A nil Loader reads a
// module as a file beside the one that loaded it.
type Source struct {
	Entry  string
	Text   []byte
	Loader Loader
}

// _Dir is the loader a script uses when it names none: a module is a file
// beside the one that loaded it.
//
// Empty: it needs no state, because the file doing the loading is an argument.
type _Dir struct{}

// LoaderFor is what reaches the modules source loads: its Loader, or files
// beside the one doing the loading when it names none.
//
// A function rather than a method, so a host holding a Source sees one name
// for what loads its modules, the field, and the two packages that read a
// script share this default without a host ever seeing it.
//
// Revisions:
//   - 2026-10-03 00:10: initial creation, as Source.Loader, replacing a default
//     in each of the two packages that read a script
//   - 2026-10-03 16:29: a function, the field taking the name Loader
func LoaderFor(source *Source) Loader {
	if source.Loader == nil {
		return new(_Dir)
	}

	return source.Loader
}

// Resolve reads target as a file beside from.
//
// A relative spelling resolves against the directory of the file that wrote it,
// so a module that moves takes its neighbours' references with it. The cleaned
// join is the name, which is what stops two spellings of one file from becoming
// two modules, and one spelling in two directories from becoming one.
//
// Revisions:
//   - 2026-09-19 20:14: initial creation
//   - 2026-09-19 20:28: resolves only; fetching moved to Load
//   - 2026-10-03 00:10: moved here from artifact, the one default a compile
//     and a derivation share
func (d *_Dir) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load reads the file at name.
//
// Revisions:
//   - 2026-09-19 20:28: initial creation
//   - 2026-10-03 00:10: moved here from artifact, the one default a compile
//     and a derivation share
func (d *_Dir) Load(name string) ([]byte, error) {
	// Not wrapped: os.ReadFile's error already names the file it could not
	// open, and the caller adds the spelling a script used.
	return os.ReadFile(name)
}
