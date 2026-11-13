// Package merge is internal, not public: it holds the merge of a script and
// its modules into one script.
//
// Package merge merges a script and every module it loads into one script
// that loads nothing.
//
// Compile uses it by default: a bundle carries one unit, the merged source,
// and no module beside it. The rules come from plan.md decision Q15 and
// poc/research/amalgamate.md:
//
//   - Modules come first, each once, dependencies before the module that loads
//     them; the entry comes last. A module runs before the entry's first
//     statement in a lark run, wherever the load line stands, so this order
//     keeps every side effect where it was.
//
//   - A load is removed. What it bound is renamed to the spelling of the global
//     it stands for.
//
//   - A name two files declare, or a file reads as a builtin, takes its
//     module's prefix on every side. A load that renames gives the global the
//     local name.
//
//   - Globals freeze once the merged top level has run and before main is
//     called, so a module may now write a value another module owns while the
//     top level runs. The unmerged run stopped with a frozen error.
//
//   - Only what a run can reach stays: main, every top-level statement with an
//     effect, and each global such a statement names, found by the resolver's
//     binding (graph/reach.go, Reach). A definition nothing reaches is cut
//     from the text and from the compiled tree. A comment is cut from the
//     text.
//
//   - Every cut keeps its lines: a def, a constant and a comment become line
//     breaks or nothing, so every line that stays keeps its number.
//
// The rewrite splices the source text and never prints a syntax tree: the
// parser library has no printer. Which identifier to splice is decided by the
// resolver's Binding, never by a spelling.
//
// Revisions:
//   - 2026-11-02 23:48: initial creation (C44a), from the proof of concept in
//     poc/research/amalgamate
//   - 2026-11-05 18:00: tree shaking and the stripping of comments (C44b)
//   - 2026-11-05 18:36: the package doc names graph.Reach, where the rule
//     lives (C44b)
//   - 2026-11-07 18:46: the package moved under v1/runtime/internal (C91w3)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
//   - 2026-11-13 17:40: package renamed from amalgam (C134, Q446)
package merge

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// UNDEFINED starts the message the resolver gives for a name nothing binds.
// go.starlark.net writes it in resolve.go, as "undefined: %s%s".
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
const (
	UNDEFINED = "undefined: "
)

var (
	// ErrExport is returned for a load of a name the module does not define
	// at its top level. The interpreter reports this only when the load runs.
	//
	// Revisions:
	//   - 2026-11-03 06:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 17:15: renamed from ErrNoExport (C121, Q348)
	ErrExport = errors.New("a module defines no such name")

	// ErrCapture is returned when substituting a loaded name would put it in
	// the scope of a local of the same spelling.
	//
	// Revisions:
	//   - 2026-11-03 06:29: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ErrCapture = errors.New("a rewritten name would be captured by a local")
)

// _Module is one parsed and resolved file.
//
// Revisions:
//   - 2026-11-05 23:24: the lines field is a *graph.Lines in place of a *_Lines (C60, Q188)
type _Module struct {
	name    string
	text    []byte
	lines   *graph.Lines
	tree    *syntax.File
	fresh   *syntax.File
	edits   []*_Edit
	globals map[string]*resolve.Binding
	order   []*resolve.Binding
	loads   []*syntax.LoadStmt
	targets []*_Module
	local   map[string]bool
	final   map[*syntax.Ident]string
	keep    []bool
}

// _Build is the state of one merge.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Build struct {
	walk  *script.Walk
	env   starlark.StringDict
	mods  map[string]*_Module
	order []*_Module
	reach []bool
}

// _Refusing is the loader of a merged script.
//
// Empty: it refuses every load, so a load left in the merge fails at the
// compile, and a bundle that is self-contained is checked, not assumed.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Refusing struct{}

// Merged is a script and every module it loads, as the trees a compile takes
// and as text.
//
// Kept is Written without the statements the shake left out: what the
// compiled tree holds, for a reader of the program's shape.
//
// Written is the merge resolved, to learn what each lambda reads; Compiled is
// the same merge as parsed, which the interpreter resolves and compiles. Every
// node in both keeps the position its author wrote it at, so a failure names
// the file and line of the module that failed. Files are the files as their
// authors wrote them, resolved, dependencies first and the entry last, for a
// check that reads what was written and not the merge.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type Merged struct {
	Written  *syntax.File
	Kept     *syntax.File
	Compiled *syntax.File
	Files    []*syntax.File
	entry    string
	pieces   []*_Piece
}

// Merge is source and every module it loads as one script that loads nothing.
//
// env is the names a compile gives the script: the runtime's plugins and those
// a host adds. A name in it cannot be a global of the merge.
//
// Every script takes the one path, with a load or without. Each file is parsed
// once, copied once and resolved once, for the bindings the renaming reads;
// the copies are renamed in place, and their statements make Compiled. When
// the script is the only file, nothing can collide or be renamed, so its
// resolved tree is Written and its copy is Compiled, and the merge costs what
// the compile of the file always cost. Otherwise Written is a resolved copy of
// Compiled.
//
// Returns the walk's refusals with their own words, the parse or resolve error
// of any file as the compile gives it, ErrExport and ErrCapture.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-02 22:58: returns the two trees a compile takes, beside the
//     text, and takes one path for every script (C44a)
//   - 2026-11-04 23:50: returns script.ErrNoSource for a nil source (C86 fix round 2)
//   - 2026-11-05 18:00: Compiled leaves out what no run reaches; Written keeps
//     it (C44b)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 17:15: uses ErrExport and ErrSource, formerly ErrNoExport and ErrNoSource (C121,
//     Q348)
//   - 2026-11-12 23:40: reads the source through script.Normalised (C130, Q405)
func Merge(
	source *common.Source,
	env starlark.StringDict,
) (*Merged, error) {
	if source == nil {
		return nil, script.ErrSource
	}

	source = script.Normalised(source)

	parsed, err := common.OPTIONS.Parse(
		source.Entry,
		source.Text,
		0,
	)
	if err != nil {
		return nil, err
	}

	build := &_Build{
		walk: script.NewWalk(
			script.LoaderFor(source),
			source.Entry,
		),
		env:  env,
		mods: map[string]*_Module{},
	}

	err = build._Added(
		source.Entry,
		source.Text,
		parsed,
	)
	if err != nil {
		return nil, err
	}

	err = build._Names(build.mods[source.Entry])
	if err != nil {
		return nil, err
	}

	return build._Merged(source.Entry)
}

// Invalid is the name of the first file whose text is not valid UTF-8, or false
// when every file is. A script runs whatever its bytes, but its text can be
// written into a bundle only as a string, which must be UTF-8.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a, plan Q51)
//   - 2026-11-04 23:50: a nil Merged holds no file, so none is invalid (C86 fix round 2)
func (m *Merged) Invalid() (string, bool) {
	if m == nil {
		return "", false
	}

	for _, piece := range m.pieces {
		if !utf8.Valid(piece.text) {
			return piece.name, true
		}
	}

	return "", false
}

// Source is the merged script as a source that loads nothing, with a loader
// that refuses every load. The text is cut and joined here, when asked, and
// not when the merge is made: a run never reads it.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from Merge
//   - 2026-11-04 23:50: a nil Merged returns script.ErrNoSource (C86 fix round 2)
//   - 2026-11-05 18:00: the text has no comment and no shaken definition (C44b)
//   - 2026-11-10 17:15: uses ErrSource, formerly ErrNoSource (C121, Q348)
func (m *Merged) Source() (*common.Source, error) {
	if m == nil {
		return nil, script.ErrSource
	}

	parts := make(
		[][]string,
		0,
		len(m.pieces),
	)

	for _, piece := range m.pieces {
		parts = append(
			parts,
			piece._Spliced(),
		)
	}

	text, err := _Rendered(parts)
	if err != nil {
		return nil, err
	}

	return &common.Source{
		Entry:  m.entry,
		Text:   text,
		Loader: new(_Refusing),
	}, nil
}

// Resolve refuses every load.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func (r *_Refusing) Resolve(
	from string,
	target string,
) (string, error) {
	return "", fmt.Errorf(
		"%s: a merged script loads nothing",
		unpack.QuotePrefix(target),
	)
}

// Load refuses every load.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func (r *_Refusing) Load(name string) ([]byte, error) {
	return nil, fmt.Errorf(
		"%s: a merged script loads nothing",
		unpack.QuotePrefix(name),
	)
}

// _Added copies and resolves the parsed tree of name, then reaches what it
// loads, so a module is added after every module it needs.
//
// The copy is taken before the resolve, which writes into its tree: the copy
// stays as parsed, and is the one the merge renames and compiles.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from _Add, which parsed name
//     itself
//   - 2026-11-05 08:07: a resolve error comes back through _Hinted (C70)
func (b *_Build) _Added(
	name string,
	text []byte,
	tree *syntax.File,
) error {
	fresh, err := dialect.Copied(tree)
	if err != nil {
		return err
	}

	err = resolve.File(
		tree,
		b.env.Has,
		starlark.Universe.Has,
	)
	if err != nil {
		return b._Hinted(err)
	}

	mod := &_Module{
		name:    name,
		text:    text,
		tree:    tree,
		fresh:   fresh,
		globals: map[string]*resolve.Binding{},
		final:   map[*syntax.Ident]string{},
	}
	b.mods[name] = mod

	for _, bind := range tree.Module.(*resolve.Module).Globals {
		mod.globals[bind.First.Name] = bind
		mod.order = append(
			mod.order,
			bind,
		)
	}

	err = b._Reached(mod)
	if err != nil {
		return err
	}

	b.order = append(
		b.order,
		mod,
	)

	return nil
}

// _Hinted is err with a suggestion on each undefined name that has none.
//
// Convention (Q158): a hint follows the failing text as `<text> (<hint>)`, in lower case.
//
// go.starlark.net suggests among the names the script binds, and says
// "(did you mean X?)". It does not look at the builtins and plugin functions,
// which are the names a typo is most often meant for. For an "undefined: name"
// error with no suggestion, this adds the one nearest name of the environment
// or the universe, in starlark's own words, so a message holds one suggestion
// at most. Any other error comes back as it is.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
func (b *_Build) _Hinted(err error) error {
	var list resolve.ErrorList
	if !errors.As(
		err,
		&list,
	) {
		return err
	}

	var names []string
	for name := range b.env {
		names = append(
			names,
			name,
		)
	}

	for name := range starlark.Universe {
		names = append(
			names,
			name,
		)
	}

	hinted := slices.Clone(list)

	for index, item := range hinted {
		name, found := strings.CutPrefix(
			item.Msg,
			UNDEFINED,
		)
		if !found || strings.Contains(
			name,
			" (",
		) {
			continue
		}

		near := spelling.Nearest(
			name,
			names,
		)
		if near != "" {
			hinted[index].Msg += " (did you mean " + near + "?)"
		}
	}

	return hinted
}

// _Reached brings in every module mod loads.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-05 23:24: the walk's refusal passes through as it is (C60, Q187)
func (b *_Build) _Reached(mod *_Module) error {
	for _, stmt := range mod.tree.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}

		name, err := b.walk.Reach(
			mod.name,
			load.Module.Value.(string),
			b._Read,
		)
		if err != nil {
			return err
		}

		mod.loads = append(
			mod.loads,
			load,
		)
		mod.targets = append(
			mod.targets,
			b.mods[name],
		)
	}

	return nil
}

// _Read parses and adds the module the walk fetched.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-02 22:58: parses the module, which _Add did
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func (b *_Build) _Read(
	name string,
	text []byte,
) error {
	tree, err := common.OPTIONS.Parse(
		name,
		text,
		0,
	)
	if err != nil {
		return err
	}

	return b._Added(
		name,
		text,
		tree,
	)
}

// _Shaken sets, in each module, the statements that no run can reach, so the
// text and the compiled tree leave them out. The rule is graph.Reach's, the
// one a derivation of the flow applies too.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, a call of Reach in place of the
//     rule that lived here (C44b, Q164)
//   - 2026-11-05 18:00: hands Reach the trees and maps (Q182)
func (b *_Build) _Shaken(entry *_Module) {
	var (
		order = make(
			[]*syntax.File,
			0,
			len(b.order),
		)
		targets = map[*syntax.LoadStmt]*syntax.File{}
		texts   = make(
			map[*syntax.File][]byte,
			len(b.order),
		)
	)

	for _, mod := range b.order {
		order = append(
			order,
			mod.tree,
		)
		texts[mod.tree] = mod.text

		for idx, load := range mod.loads {
			targets[load] = mod.targets[idx].tree
		}
	}

	keeps := graph.Reach(
		entry.tree,
		order,
		targets,
		texts,
	)

	for _, mod := range b.order {
		mod.keep = keeps[mod.tree]
	}
}
