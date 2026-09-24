package artifact

import (
	"context"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/runtime/dialect"
	"github.com/thebagchi/lark/runtime/guard"
	"github.com/thebagchi/lark/runtime/plugin"
)

// Loader fetches the source of a module a script asked to load, and says what
// that module is actually called.
//
// The interface is declared here rather than borrowed so that what this package
// depends on is the single method it calls. A host serving scripts from a
// directory, an archive, a database or a test map implements this without
// having to be, or to own, a file system.
//
// Resolving is separate from fetching, and the split is what keeps two
// properties the one-method shape gave up: a module reached by two routes is
// fetched once, and a cycle is refused without fetching anything. Both need the
// graph to know what a spelling means before it decides whether to ask for it.
//
// Resolve is given the module doing the loading and the spelling it used, and
// returns the identity the artifact files that module under. Two spellings that
// reach one module must resolve to one name, or it is built twice; one spelling
// that reaches two modules must resolve to two, or the wrong one is reused. It
// is expected to be cheap - a path join, a key normalisation - because it runs
// for every load whether or not a fetch follows.
type Loader interface {
	Resolve(from string, path string) (string, error)
	Load(name string) ([]byte, error)
}

// _Dir is the loader a Compiler uses when the host set none: a module is a file
// beside the one that loaded it.
//
// Empty: it needs no state, because the file doing the loading is an argument.
type _Dir struct{}

// _Graph collects the compiled units of one artifact, in an order that puts a
// dependency before whatever loads it.
//
// source is what each unit was compiled from, kept so that describing the
// artifact afterwards reads no file twice. Every module is fetched once per
// compile, and deriving a graph through the loader again would have broken
// that - measured, by the test that counts what the loader was asked for.
type _Graph struct {
	loader   Loader
	registry *plugin.Registry
	env      starlark.StringDict
	units    map[string]*_Unit
	order    []string
	chain    []string
	source   map[string][]byte
}

// _Held serves the sources a compile already read.
//
// It is a graph.Source, which asks exactly what a Loader does, so describing
// an artifact costs no fetch and cannot read a file that has changed since the
// compile. Resolving still goes through the loader, which is a path join
// rather than a read.
type _Held struct {
	loader Loader
	source map[string][]byte
}

// Resolve asks the loader what a spelling means, or reads it as a file beside
// the one that loaded it when there is no loader.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func (h *_Held) Resolve(from string, target string) (string, error) {
	if h.loader == nil {
		return (&_Dir{}).Resolve(from, target)
	}

	return h.loader.Resolve(from, target)
}

// Load returns what the compile read for this module.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func (h *_Held) Load(name string) ([]byte, error) {
	src, held := h.source[name]
	if !held {
		return nil, fmt.Errorf("%s: %w", name, ErrNoUnit)
	}

	return src, nil
}

// Resolve reads target as a file beside from.
//
// A relative spelling resolves against the directory of the file that wrote it,
// so a module that moves takes its neighbours' references with it. The cleaned
// join is the name, which is what stops two spellings of one file from becoming
// two units, and one spelling in two directories from becoming one.
//
// Revisions:
//   - 2026-09-19 20:14: initial creation
//   - 2026-09-19 20:28: resolves only; fetching moved to Load
func (d *_Dir) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load reads the file at name.
//
// Revisions:
//   - 2026-09-19 20:28: initial creation
func (d *_Dir) Load(name string) ([]byte, error) {
	// Not wrapped: os.ReadFile's error already names the file it could not
	// open, and the caller adds the spelling a script used.
	return os.ReadFile(name)
}

// _Add compiles src as path, then walks whatever it loads, depth first.
//
// A compiled program lists its loads without having been run, which is what
// lets the whole graph be built, and a cycle be refused, before any top-level
// statement executes. The order is built on the way out of the recursion, so a
// dependency always precedes whatever loaded it.
//
// Revisions:
//   - 2026-09-19 18:30: initial creation
//   - 2026-09-21 17:19: keeps what it compiled, so describing reads nothing
//     twice
func (g *_Graph) _Add(path string, src []byte) (*_Unit, error) {
	tree, code, err := starlark.SourceProgramOptions(
		dialect.OPTIONS,
		path,
		src,
		g.env.Has,
	)
	if err != nil {
		// Not wrapped with the path: a parse or resolve error from the
		// interpreter already opens with file:line:column, and repeating the
		// file makes a reader scan past it twice to reach the position.
		return nil, err
	}

	var encoded strings.Builder

	err = code.Write(&encoded)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", path, err)
	}

	// Not wrapped with the path: a refusal carries the position it was
	// written at, which opens with the file, and repeating it makes a reader
	// scan past the same name twice.
	err = _Checked(g.registry, tree)
	if err != nil {
		return nil, err
	}

	g.source[path] = src

	unit := &_Unit{
		saved: &artifactpb.Unit{
			Name:  path,
			Code:  []byte(encoded.String()),
			Loads: map[string]string{},
		},
		tree: tree,
		code: code,
	}

	g.units[path] = unit

	for index := range code.NumLoads() {
		module, _ := code.Load(index)

		name, err := g._Reach(path, module)
		if err != nil {
			return nil, err
		}

		unit.saved.Loads[module] = name
	}

	g.order = append(g.order, path)

	return unit, nil
}

// _Reach brings the module target, as spelled by from, into the graph if it is
// not there already.
//
// The loader is asked to resolve first, because until it says what target is
// called there is nothing to compare: one spelling can reach two modules and
// two spellings can reach one, and only the loader knows which. Everything
// after that is keyed on the name it returned, and the fetch happens last, so
// a module already built and a module that closes a cycle both cost nothing to
// read.
//
// The chain is then checked before the map of units, and that order is the
// rule: _Add registers a unit as soon as it compiles and before walking its own
// loads, so everything on the chain is already in that map. Asking the map
// first answers "seen it" for a unit that is still being built, which is
// exactly the case a cycle is.
//
// Revisions:
//   - 2026-09-19 18:32: initial creation
//   - 2026-09-19 18:57: checks the chain before the map, so a cycle through the
//     entry is refused rather than reported as a missing unit at link time
//   - 2026-09-19 20:18: resolves through the loader before either check, so a
//     module is identified by what it is rather than by how it was spelled
//   - 2026-09-19 20:28: fetches only after both checks, restoring one fetch per
//     module and none at all for a cycle
func (g *_Graph) _Reach(from string, target string) (string, error) {
	if g.loader == nil {
		return "", fmt.Errorf("load %s: %w", target, ErrNoLoader)
	}

	name, err := g.loader.Resolve(from, target)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q from %s: %w", target, from, err)
	}

	start := slices.Index(g.chain, name)
	if start >= 0 {
		ring := append(slices.Clone(g.chain[start:]), name)

		return "", fmt.Errorf("%w: %s", ErrCycle, strings.Join(ring, CHAIN_ARROW))
	}

	_, done := g.units[name]
	if done {
		return name, nil
	}

	src, err := g.loader.Load(name)
	if err != nil {
		// The spelling and the file that wrote it, because the loader's own
		// error says what it tried to reach. Wrapping with the resolved name
		// would print one path twice.
		return "", fmt.Errorf("cannot load %q from %s: %w", target, from, err)
	}

	g.chain = append(g.chain, name)

	defer func() {
		g.chain = g.chain[:len(g.chain)-1]
	}()

	_, err = g._Add(name, src)

	return name, err
}

// _Link assembles the units into one artifact, in the order that initialises
// dependencies first.
//
// Nothing runs here. A script's module-level statements execute once per run,
// not once per compile, because what they produce includes the arguments the
// run supplies - and a value bound at compile time would be one compile's
// value shared by every run of that artifact.
//
// env is the one the source was compiled against, kept rather than rebuilt at
// each run. A script resolved against one environment and initialised against
// another is a script whose names exist at compile time and not at run time -
// and a plugin holding state would hand out a different store to each.
//
// Revisions:
//   - 2026-09-19 18:36: initial creation
//   - 2026-09-22 22:24: assembles without initialising, which moved to the run
func _Link(entry string, env starlark.StringDict, units map[string]*_Unit, order []string) *Artifact {
	message := &artifactpb.Artifact{
		Entry: entry,
		Units: make([]*artifactpb.Unit, 0, len(order)),
	}

	for _, name := range order {
		message.Units = append(message.Units, units[name].saved)
	}

	return &Artifact{
		saved: message,
		units: units,
		env:   env,
		order: order,
	}
}

// _Initialise runs every unit's module-level statements in order, with this
// run's arguments bound on each thread, and freezes what each produced.
//
// Once per run. Dependencies come first, so the load hook only ever has to
// hand back globals that are already built and frozen - there is nothing left
// to fetch by the time anything runs.
//
// Initialising is guarded, because a module's top level runs arbitrary script
// and a script must not be able to take the host down with it.
//
// Returns what the entry unit produced, and a wrapped error if any unit fails
// to initialise. Never panics.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation, holding what _Link used to do
func (a *Artifact) _Initialise(ctx context.Context) (starlark.StringDict, error) {
	built := map[string]starlark.StringDict{}

	var err error

	for _, path := range a.order {
		unit := a.units[path]

		load := func(thread *starlark.Thread, spelling string) (starlark.StringDict, error) {
			globals, found := built[unit.saved.GetLoads()[spelling]]
			if !found {
				return nil, fmt.Errorf("%s: %w", spelling, ErrNoUnit)
			}

			return globals, nil
		}

		thread := &starlark.Thread{
			Name: path,
			Load: load,
		}

		_Bind(thread, ctx)

		var globals starlark.StringDict

		guard.WithRecover(
			&globals,
			&err,
			func() (starlark.StringDict, error) {
				return unit.code.Init(thread, a.env)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("initialise %s: %w", path, err)
		}

		globals.Freeze()

		built[path] = globals
	}

	return built[a.saved.GetEntry()], nil
}

// _Checked lets every plugin that wants to refuse a use of its own names read
// the source before anything runs.
//
// The compiler asks and does not look: which names mean what is the plugin's
// business, and a compiler that knew would be one more thing to change every
// time somebody writes a plugin.
//
// Revisions:
//   - 2026-09-24 17:40: initial creation
func _Checked(registry *plugin.Registry, tree *syntax.File) error {
	if registry == nil {
		return nil
	}

	for _, installed := range registry.Registered() {
		checker, ok := installed.(plugin.Checking)
		if !ok {
			continue
		}

		err := checker.Check(tree)
		if err != nil {
			return err
		}
	}

	return nil
}
