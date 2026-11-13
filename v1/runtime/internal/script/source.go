// Package script is internal, not public: it holds a script as the runtime
// reads it, the main script and the loader of its modules.
//
// Package script is a script as the runtime reads it: the main script's name
// and text, and what reaches the modules it loads.
//
// Its own package because two sides read one script: a compile, which builds an
// artifact from it, and a derivation, which builds the flow it describes. Each
// declaring its own shape would be two declarations of one thing, and a host
// holding a script would convert between them.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:30: moved under v1/runtime/internal; Source and Loader moved to common (C91w2)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
package script

import (
	"bytes"
	"errors"
	"os"
	"path"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// ErrSource is returned where a script was wanted and a nil one was given: a
// host that passed nothing is told so, and its process is left alone.
//
// Revisions:
//   - 2026-11-05 00:19: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: renamed from ErrNoSource (C121, Q348)
var (
	ErrSource = errors.New("no source to compile")
)

// _Dir is the loader a script uses when it names none: a module is a file
// beside the one that loaded it.
//
// Empty: it needs no state, because the file doing the loading is an argument.
//
// Revisions:
//   - 2026-09-19 23:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
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
//   - 2026-11-04 23:50: a nil source reads files beside the loading one (C86 fix round 2)
//   - 2026-11-07 18:30: takes and returns the types in common (C91w2)
func LoaderFor(source *common.Source) common.Loader {
	if source == nil || source.Loader == nil {
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
func (d *_Dir) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
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

// Normal is text with each lone carriage return, one that no line feed
// follows, written as a line feed. The text itself is returned when it has none.
// A carriage return and a line feed together stay as they are, and so does a NUL.
//
// go.starlark.net reads a carriage return, a carriage return and a line feed,
// and a line feed as one line break, and the value of a string over several
// lines holds a line feed for each (scan.go readRune, quote.go Unquote), so the
// lines and the values of a script do not change. Every other reader of the text
// counts line feeds only, so a script that the scanner reads in lines and they
// read in one would be cut and shaken in the wrong places (Q405).
//
// Revisions:
//   - 2026-11-12 23:39: initial creation (C130, Q405)
func Normal(text []byte) []byte {
	var out []byte

	for at, char := range text {
		if char != '\r' || at+1 < len(text) && text[at+1] == '\n' {
			continue
		}

		if out == nil {
			out = bytes.Clone(text)
		}

		out[at] = '\n'
	}

	if out == nil {
		return text
	}

	return out
}

// _Normal is a loader that gives the text of every module through Normal.
//
// Revisions:
//   - 2026-11-12 23:39: initial creation (C130, Q405)
type _Normal struct {
	inner common.Loader
}

// Resolve is what the loader under it resolves.
//
// Revisions:
//   - 2026-11-12 23:39: initial creation (C130, Q405)
func (n *_Normal) Resolve(
	from string,
	target string,
) (string, error) {
	return n.inner.Resolve(
		from,
		target,
	)
}

// Load is the text the loader under it loads, through Normal.
//
// Revisions:
//   - 2026-11-12 23:39: initial creation (C130, Q405)
func (n *_Normal) Load(name string) ([]byte, error) {
	text, err := n.inner.Load(name)
	if err != nil {
		return nil, err
	}

	return Normal(text), nil
}

// Normalised is source with its text, and the text of every module it loads,
// through Normal. It is the one place a source enters the runtime: a compile and
// a derivation each take their source through it first, so no reader sees a lone
// carriage return and the script is shaken the same way by both (Q405). A nil
// source stays nil, and the source given is not changed.
//
// Revisions:
//   - 2026-11-12 23:39: initial creation (C130, Q405)
func Normalised(source *common.Source) *common.Source {
	if source == nil {
		return nil
	}

	return &common.Source{
		Entry: source.Entry,
		Text:  Normal(source.Text),
		Loader: &_Normal{
			inner: LoaderFor(source),
		},
	}
}
